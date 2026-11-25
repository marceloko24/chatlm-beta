//go:build js && wasm

// ChatLM drives llama.cpp through yzma in a Web Worker.
package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"syscall/js"
	"time"
	"unicode/utf8"

	"github.com/hybridgroup/yzma/pkg/llamawasm"
	"github.com/hybridgroup/yzma/pkg/message"
	"github.com/hybridgroup/yzma/pkg/template"
)

const (
	modelPath        = "/models/model.gguf"
	defaultSystem    = "Sos un asistente útil. Respondé en español, de forma breve y clara."
	nCtx             = 512
	nBatch           = 512
	nUbatch          = 512
	defaultMaxTokens = 128
)

type turn struct {
	Role    string
	Content string
}

var (
	model   llamawasm.Model
	ctx     llamawasm.Context
	vocab   llamawasm.Vocab
	sampler llamawasm.Sampler

	history = []turn{{Role: "system", Content: defaultSystem}}

	stopping atomic.Bool
	opening  atomic.Bool
	asking   atomic.Bool
	loadedAt string

	// noThink is set only for qwen3, so the official chat template is
	// rendered with enable_thinking=false. Qwen2.5 and other models keep
	// the previous ChatApplyTemplate path unchanged.
	noThink bool
	chatTmpl string
)

func main() {
	if err := llamawasm.Load(""); err != nil {
		post("error", "No se pudo iniciar el motor: "+err.Error())
		return
	}

	llamawasm.LogSet(llamawasm.LogSilent())
	llamawasm.Init()

	js.Global().Set("yzmaOpenModel", js.FuncOf(openModel))
	js.Global().Set("yzmaAsk", js.FuncOf(ask))
	js.Global().Set("yzmaStop", js.FuncOf(stop))
	js.Global().Set("yzmaReset", js.FuncOf(reset))
	js.Global().Set("yzmaSetSystem", js.FuncOf(setSystem))

	post("system", defaultSystem)
	post("ready", backendReport())

	// Keep the runtime alive. If main returns, later resume() calls trap.
	select {}
}

func openModel(this js.Value, args []js.Value) any {
	path := modelPath
	if len(args) > 0 && args[0].Truthy() {
		path = args[0].String()
	}
	// identity distinguishes two GGUFs that share the same memfs path.
	identity := path
	if len(args) > 1 && args[1].Truthy() {
		identity = args[1].String()
	}
	go open(path, identity)
	return nil
}

func open(path, identity string) {
	if asking.Load() {
		post("error", "Esperá a que termine la respuesta, o pulsá Parar, antes de volver a cargar el modelo.")
		return
	}
	if !opening.CompareAndSwap(false, true) {
		return
	}
	defer opening.Store(false)

	// Reloading the same file into a live instance is what often trips
	// TinyGo's asyncify scheduler (resume() / unreachable). A different
	// identity (name and size) must load, even if the memfs path is the same.
	if model != 0 && ctx != 0 && loadedAt == identity {
		post("loaded", modelLabel())
		return
	}

	post("status", "Cargando el modelo en el motor…")

	freeSampler()
	if ctx != 0 {
		_ = llamawasm.Free(ctx)
		ctx = 0
	}
	if model != 0 {
		_ = llamawasm.ModelFree(model)
		model = 0
	}
	loadedAt = ""

	params := llamawasm.ModelDefaultParams()
	var err error
	if model, err = llamawasm.ModelLoadFromFile(path, params); err != nil {
		post("error", "No se pudo cargar el modelo: "+err.Error())
		return
	}

	ctxParams := llamawasm.ContextDefaultParams()
	ctxParams.NCtx = nCtx
	ctxParams.NBatch = nBatch
	ctxParams.NUbatch = nUbatch
	ctxParams.NThreads = 1

	if ctx, err = llamawasm.InitFromModel(model, ctxParams); err != nil {
		_ = llamawasm.ModelFree(model)
		model = 0
		post("error", "No se pudo crear el contexto: "+err.Error())
		return
	}

	vocab = llamawasm.ModelGetVocab(model)
	freeSampler()
	history = history[:1]
	loadedAt = identity
	noThink = false
	chatTmpl = ""

	tmpl := llamawasm.ModelChatTemplate(model, "")
	if tmpl == "" {
		if meta, ok := llamawasm.ModelMetaValStr(model, "tokenizer.chat_template"); ok {
			tmpl = meta
		}
	}
	if tmpl == "" {
		post("status", "Este modelo no trae plantilla de chat; las respuestas pueden ser flojas.")
	} else if modelArch() == "qwen3" {
		chatTmpl = tmpl
		noThink = true
	}

	post("loaded", modelLabel())
}

func modelLabel() string {
	desc := strings.TrimSpace(llamawasm.ModelDesc(model))
	size := formatParams(llamawasm.ModelNParams(model))
	arch, quant := splitDesc(desc)
	head := strings.TrimSpace(strings.Join([]string{arch, size, quant}, " "))
	if head == "" {
		head = desc
	}
	if head == "" {
		return backendReport()
	}
	return head + ", " + backendReport()
}

// splitDesc pulls architecture and quantization out of llama.cpp's
// ModelDesc (e.g. "qwen2 1B Q4_K - Medium"). The "1B" token is a bucket,
// not the real parameter count, so the caller replaces it with ModelNParams.
func splitDesc(desc string) (arch, quant string) {
	fields := strings.Fields(desc)
	var rest []string
	for _, f := range fields {
		if isSizeBucket(f) {
			continue
		}
		if quant == "" && isQuantToken(f) {
			quant = f
			continue
		}
		if quant != "" {
			// Keep " - Medium" attached to the quant.
			quant = strings.TrimSpace(quant + " " + f)
			continue
		}
		rest = append(rest, f)
	}
	return strings.Join(rest, " "), quant
}

func isSizeBucket(f string) bool {
	if len(f) < 2 {
		return false
	}
	unit := f[len(f)-1]
	if unit != 'B' && unit != 'M' && unit != 'b' && unit != 'm' {
		return false
	}
	for _, c := range f[:len(f)-1] {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func isQuantToken(f string) bool {
	s := strings.ToUpper(f)
	if strings.HasPrefix(s, "IQ") {
		s = s[2:]
	} else if len(s) > 0 && (s[0] == 'Q' || s[0] == 'F' || s[0] == 'I') {
		s = s[1:]
	} else {
		return false
	}
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

func formatParams(n uint64) string {
	if n == 0 {
		return ""
	}
	if n >= 1_000_000_000 {
		v := float64(n) / 1e9
		if v >= 10 {
			return fmt.Sprintf("%.0fB", v)
		}
		return fmt.Sprintf("%.1fB", v)
	}
	if n >= 100_000_000 {
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%dM", (n+500_000)/1_000_000)
	}
	return fmt.Sprintf("%d", n)
}

func makeSampler() {
	freeSampler()
	sampler = llamawasm.SamplerChainInit(llamawasm.SamplerChainDefaultParams())
	llamawasm.SamplerChainAdd(sampler, llamawasm.SamplerInitPenalties(llamawasm.VocabNTokens(vocab), 64, 1.1, 0, 0))
	llamawasm.SamplerChainAdd(sampler, llamawasm.SamplerInitTopK(40))
	llamawasm.SamplerChainAdd(sampler, llamawasm.SamplerInitTopP(0.95, 1))
	llamawasm.SamplerChainAdd(sampler, llamawasm.SamplerInitTemp(0.7))
	llamawasm.SamplerChainAdd(sampler, llamawasm.SamplerInitDist(uint32(time.Now().UnixNano())))
}

func freeSampler() {
	if sampler != 0 {
		llamawasm.SamplerFree(sampler)
		sampler = 0
	}
}

func backendReport() string {
	return fmt.Sprintf("CPU, 1 hilo, contexto %d", nCtx)
}

func setSystem(this js.Value, args []js.Value) any {
	text := defaultSystem
	if len(args) > 0 && args[0].Truthy() {
		if trimmed := strings.TrimSpace(args[0].String()); trimmed != "" {
			text = trimmed
		}
	}
	history[0] = turn{Role: "system", Content: text}
	return nil
}

func reset(this js.Value, args []js.Value) any {
	stopping.Store(true)
	history = history[:1]
	post("reset", "")
	return nil
}

func stop(this js.Value, args []js.Value) any {
	stopping.Store(true)
	return nil
}

func ask(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		post("error", "Falta la pregunta.")
		return nil
	}
	question := args[0].String()

	maxTokens := int32(defaultMaxTokens)
	if len(args) > 1 && args[1].Truthy() {
		maxTokens = int32(args[1].Int())
	}

	if opening.Load() {
		post("error", "El modelo todavía se está cargando.")
		return nil
	}
	if model == 0 || ctx == 0 {
		post("error", "Primero hay que cargar un modelo.")
		return nil
	}
	if !asking.CompareAndSwap(false, true) {
		post("error", "Ya estoy generando una respuesta.")
		return nil
	}

	go func() {
		defer asking.Store(false)
		stopping.Store(false)

		history = append(history, turn{Role: "user", Content: question})

		tokens := prompt(maxTokens)
		if len(tokens) == 0 {
			post("error", "El prompt no produjo tokens.")
			return
		}

		if err := llamawasm.MemoryClear(ctx, true); err != nil {
			post("error", err.Error())
			return
		}
		makeSampler()

		var (
			reply strings.Builder
			count int32
			hold  string
		)
		batch := llamawasm.BatchGetOne(tokens)
		buf := make([]byte, 64)
		start := time.Now()

		for count < maxTokens && !stopping.Load() {
			if _, err := llamawasm.Decode(ctx, batch); err != nil {
				post("error", "Error al generar: "+err.Error())
				return
			}

			token := llamawasm.SamplerSample(sampler, ctx, -1)
			if llamawasm.VocabIsEOG(vocab, token) {
				break
			}

			if n := llamawasm.TokenToPiece(vocab, token, buf, 0, true); n > 0 {
				hold += string(buf[:n])
				// Hold back a few bytes so a multibyte character is not split.
				if keep := 3; len(hold) > keep {
					cut := len(hold) - keep
					for cut > 0 && !utf8.RuneStart(hold[cut]) {
						cut--
					}
					if cut > 0 {
						piece := hold[:cut]
						hold = hold[cut:]
						reply.WriteString(piece)
						post("token", piece)
					}
				}
			}

			count++
			batch = llamawasm.BatchGetOne([]llamawasm.Token{token})
			yield()
		}

		if hold != "" {
			reply.WriteString(hold)
			post("token", hold)
		}

		history = append(history, turn{Role: "assistant", Content: reply.String()})

		note := ""
		if stopping.Load() {
			note = ", detenido"
		}
		if count == 0 && !stopping.Load() {
			post("error", "El modelo no generó ninguna respuesta.")
			return
		}

		if elapsed := time.Since(start).Seconds(); elapsed > 0 && count > 0 {
			post("done", fmt.Sprintf("%d tokens, %.1f tokens/s%s", count, float64(count)/elapsed, note))
			return
		}
		post("done", fmt.Sprintf("%d tokens%s", count, note))
	}()

	return nil
}

func prompt(maxTokens int32) []llamawasm.Token {
	for {
		tokens := tokenizePrompt()
		if len(tokens) == 0 {
			return nil
		}

		if int32(len(tokens)) <= nCtx-maxTokens || len(history) <= 2 {
			return tokens
		}

		drop := 2
		if len(history) < 1+drop {
			drop = len(history) - 1
		}
		history = append(history[:1], history[1+drop:]...)
	}
}

func tokenizePrompt() []llamawasm.Token {
	if noThink && chatTmpl != "" {
		text, err := renderQwen3NoThink()
		if err != nil {
			post("error", "No se pudo armar el prompt sin pensamiento: "+err.Error())
			return nil
		}
		if text == "" {
			post("error", "El prompt no produjo texto.")
			return nil
		}
		return llamawasm.Tokenize(vocab, text, true, true)
	}

	var text strings.Builder
	for i, turn := range history {
		formatted, err := llamawasm.ChatApplyTemplate(model, turn.Role, turn.Content, i == len(history)-1)
		if err != nil {
			return llamawasm.Tokenize(vocab, history[len(history)-1].Content, true, false)
		}
		text.WriteString(formatted)
	}
	return llamawasm.Tokenize(vocab, text.String(), true, true)
}

func modelArch() string {
	if s, ok := llamawasm.ModelMetaValStr(model, "general.architecture"); ok {
		if arch := strings.ToLower(strings.TrimSpace(s)); arch != "" {
			return arch
		}
	}
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(llamawasm.ModelDesc(model))))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// renderQwen3NoThink applies the GGUF jinja template with enable_thinking=false,
// which inserts the official empty <think></think> assistant prefix. It does
// not strip reasoning text from the model's reply.
func renderQwen3NoThink() (string, error) {
	msgs := make([]message.Message, 0, len(history))
	for _, turn := range history {
		msgs = append(msgs, message.Chat{Role: turn.Role, Content: turn.Content})
	}
	text, err := template.ApplyWithOptions(chatTmpl, msgs, true, template.Options{EnableThinking: false})
	if err == nil && text != "" {
		return text, nil
	}

	// Fallback if the jinja engine cannot render this template: ChatApplyTemplate
	// cannot pass enable_thinking, so append the official empty block from the
	// GGUF template. Only used for qwen3.
	var b strings.Builder
	for i, turn := range history {
		formatted, err2 := llamawasm.ChatApplyTemplate(model, turn.Role, turn.Content, i == len(history)-1)
		if err2 != nil {
			if err != nil {
				return "", err
			}
			return "", err2
		}
		b.WriteString(formatted)
	}
	text = b.String()
	official := "<think>\n\n</think>"
	if !strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), official) {
		text += official + "\n\n"
	}
	return text, nil
}

func post(kind, text string) {
	message := map[string]any{"kind": kind, "text": text}
	if fn := js.Global().Get("postMessage"); fn.Type() == js.TypeFunction {
		fn.Invoke(message)
		return
	}
	fmt.Printf("%s: %s\n", kind, text)
}

// yield returns to the JavaScript event loop so Parar and token painting work.
// TinyGo's wasm target uses the asyncify scheduler. A setTimeout + js.FuncOf
// callback calls resume(); mixing that with the scheduler's go_scheduler()
// wakeups is what surfaces as "Unreachable code ... resume()". time.Sleep
// uses the official sleepTicks path instead.
func yield() {
	time.Sleep(time.Millisecond)
}
