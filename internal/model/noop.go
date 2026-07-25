package model

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type NoopProvider struct {
	DefaultModel string
	// Locale is the UI's language ("th", "en"). This provider is the one place
	// in the engine that needs it: it is not a model, it is the screen a user
	// with nothing configured is talking to (ARCHITECTURE.md §40). Empty falls
	// back to Thai.
	Locale string
}

// noopOnboardingReply is what a genuinely unconfigured install sees on every
// turn — a real user typing into a fresh Aetox has no API key set anywhere,
// so every chat request lands here (see config.Load: provider defaults to
// "aetox") until they visit Settings. It replaces what used to be a raw
// "[noop:model] text" debug echo, which no first-time user should ever see.
//
// One per language, picked by NoopProvider.Locale (§40). The guide chips that
// follow this reply live in the frontend locale files (§39); this one cannot,
// because it is a model reply, so the locale is carried to it instead.
const noopOnboardingReply = `สวัสดีครับ Aetox ยังไม่ได้เชื่อมต่อกับโมเดลจริง

**ทำไมถึงเป็นแบบนี้**
Aetox เกิดจากความคิดของนักพัฒนาเพียงคนเดียว ไม่มีทีม ไม่มีบริษัท เราจึงไม่สามารถหาโมเดลฟรีมาให้บริการได้จริงๆ

**ต้องทำอะไรต่อ**
ไปที่หน้า ตั้งค่า (Settings) แล้วเลือกผู้ให้บริการที่คุณไว้ใจ เพื่อตั้งเป็นเครื่องยนต์ให้ Aetox ได้เลยครับ

**วิสัยทัศน์ของเรา**
แม้ว่าทางเราจะไม่มีทุน แต่เรามีวิสัยทัศน์ ผู้พัฒนาเล็งเห็นว่า หัวใจไม่ใช่ความรู้ในโมเดล แต่คือ Architecture ที่ควบคุมวิธีคิด แต่เราไม่มีทุนในการเทรนโมเดลใหม่เอง จำใจต้องใช้วิธีนี้

(Aetox isn't connected to a real model yet — open Settings and pick a provider you trust to power it.)`

const noopOnboardingReplyEN = `Hi — Aetox isn't connected to a real model yet.

**Why**
Aetox is built by one developer. No team, no company, and no budget to hand out free model access.

**What to do**
Open **Settings → Model settings**, pick a provider you trust, and it powers Aetox from there. Want nothing leaving your machine? Choose **Ollama** or **LM Studio** and run the model locally — no key, and not a byte of your prompts leaves the machine.

**The bet behind this**
What makes an agent useful is not the knowledge packed into the model, it is the architecture governing how it works. There was no funding to train a model, so the bet went on architecture — and on letting you plug in whichever model you like.`

// onboardingReply picks the language the UI is showing. Anything that isn't
// English falls back to Thai, which is what a fresh install runs in.
func (p *NoopProvider) onboardingReply() string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Locale)), "en") {
		return noopOnboardingReplyEN
	}
	return noopOnboardingReply
}

func NewNoopProvider(model string) *NoopProvider {
	return &NoopProvider{DefaultModel: model}
}

func (p *NoopProvider) Name() string {
	return "aetox"
}

func (p *NoopProvider) SupportsToolCalling() bool {
	// aetox-tools:test opts into the real tool loop so the tool-driven UI
	// (todo panel, ask_user cards, tool timeline) is exercisable without a key.
	return strings.Contains(strings.ToLower(p.DefaultModel), "tools")
}

func (p *NoopProvider) SupportsReasoning() bool {
	return false
}

func (p *NoopProvider) Complete(_ context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.DefaultModel
	}
	if model == "" {
		model = "aetox"
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	text := strings.TrimSpace(lastMessage.Content)
	if text == "" {
		text = "(empty prompt)"
	}

	// Test models: the picked model name decides the response shape, so each
	// chat-rendering path can be exercised on its own (see provider catalog).
	modelKey := strings.ToLower(model)
	switch {
	case strings.Contains(modelKey, "image"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     noopImageReply(text),
		}, nil
	case strings.Contains(modelKey, "think"):
		return Response{
			Provider:         p.Name(),
			Model:            model,
			ReasoningContent: noopLongReasoning(text),
			Text:             "[think-test] คำตอบสั้น ๆ หลังคิดเสร็จ: " + clipNoop(text, 80),
		}, nil
	case strings.Contains(modelKey, "markdown"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     noopMarkdownReply(),
		}, nil
	case strings.Contains(modelKey, "tools"):
		return p.noopToolsReply(model, req), nil
	}

	if scripted, ok := noopScenario(text); ok {
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     scripted,
		}, nil
	}

	return Response{
		Provider: p.Name(),
		Model:    model,
		Text:     p.onboardingReply(),
	}, nil
}

// noopImageReply: any prompt gets the full research-style gallery; the img*
// keywords still pick a specific case (single, wrap, huge, broken).
func noopImageReply(text string) string {
	if scripted, ok := noopScenario(text); ok {
		return scripted
	}
	scripted, _ := noopScenario("imgmix")
	return scripted
}

func noopMarkdownReply() string {
	return "## ทดสอบ Markdown ครบชุด\n\n" +
		"ย่อหน้าปกติ **ตัวหนา** *ตัวเอียง* `inline code` และ[ลิงก์](https://example.com)\n\n" +
		"```go\nfunc main() {\n\tfmt.Println(\"code block\")\n}\n```\n\n" +
		"| คอลัมน์ | ค่า |\n|---|---|\n| หนึ่ง | 111 |\n| สอง | 222 |\n\n" +
		"1. รายการเรียงลำดับ\n2. ข้อสอง\n\n- รายการจุด\n- ข้อสอง\n\n> คำพูดยกมา (blockquote)\n\n---\n\nจบชุดทดสอบครับ"
}

// noopToolsReply scripts one fixed tool-using turn so the tool-driven UI can
// be exercised with no API key: switch to aetox-tools:test and send anything.
// The round is derived from the tool results already in the transcript —
// stateless, so it survives re-bootstraps mid-conversation:
//  1. todo_write (checklist appears, one item in_progress)
//  2. ask_user   (the option cards block until the user picks)
//  3. todo_write (all items completed)
//  4. final text echoing the user's choice
func (p *NoopProvider) noopToolsReply(model string, req Request) Response {
	todoCalls, askCalls := 0, 0
	lastAnswer := ""
	for _, m := range req.Messages {
		if m.Role != RoleTool {
			continue
		}
		switch m.Name {
		case "todo_write":
			todoCalls++
		case "ask_user":
			askCalls++
			lastAnswer = strings.TrimSpace(m.Content)
		}
	}
	call := func(id, name, args string) Response {
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID:   id,
			Type: "function",
			Function: FunctionCall{Name: name, Arguments: args},
		}}}
	}
	switch {
	case todoCalls == 0:
		return call("noop_todo_1", "todo_write",
			`{"todos":[{"content":"วางแผนชุดทดสอบ UI","status":"completed"},{"content":"แสดง checklist ระหว่างทำงาน","status":"in_progress"},{"content":"ถามผู้ใช้ด้วย ask_user","status":"pending"},{"content":"สรุปผลการทดสอบ","status":"pending"}]}`)
	case askCalls == 0:
		return call("noop_ask_1", "ask_user",
			`{"question":"ทดสอบ ask_user: อยากให้ตอบกลับด้วยโทนไหนครับ?","options":["สั้น กระชับ","ละเอียด ยกตัวอย่าง","ขำๆ มีอีโมจิ","ทางการ"]}`)
	case todoCalls == 1:
		return call("noop_todo_2", "todo_write",
			`{"todos":[{"content":"วางแผนชุดทดสอบ UI","status":"completed"},{"content":"แสดง checklist ระหว่างทำงาน","status":"completed"},{"content":"ถามผู้ใช้ด้วย ask_user","status":"completed"},{"content":"สรุปผลการทดสอบ","status":"completed"}]}`)
	default:
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text: "✅ จบชุดทดสอบ tools UI ครับ — todo panel, ask_user cards และ tool timeline ทำงานครบ\n\n" +
				"ผลจาก ask_user: " + lastAnswer,
		}
	}
}

// noopLongReasoning produces a multi-paragraph thinking stream (~2 minutes of
// word-by-word trickle) so the live reasoning panel, its unbounded height, the
// pinned auto-scroll, and the collapsed "done thinking" toggle all get a real
// workout without an API key.
func noopLongReasoning(text string) string {
	sections := []struct{ head, body string }{
		{"ตีโจทย์", "ผู้ใช้ถามว่า \"" + clipNoop(text, 60) + "\" — ก่อนอื่นต้องแยกให้ออกว่านี่คือคำถามเชิงข้อเท็จจริง เชิงความเห็น หรือเป็นคำสั่งให้ลงมือทำ เพราะโครงคำตอบต่างกันมาก ถ้าตีความผิดตั้งแต่ต้น ที่เหลือจะเพี้ยนหมด"},
		{"แตกประเด็น", "ลองแตกออกเป็นสามแกน: (1) สิ่งที่ผู้ใช้พูดตรง ๆ (2) สิ่งที่น่าจะอยากได้จริง ๆ แต่ไม่ได้พูด (3) ข้อจำกัดแวดล้อม เช่น เวลา บริบทก่อนหน้าในบทสนทนา และรูปแบบคำตอบที่เหมาะ ประเด็นที่สองสำคัญสุดและพลาดง่ายสุด"},
		{"ตั้งสมมติฐาน", "สมมติฐานแรก: ตอบสั้นตรงประเด็นพอ สมมติฐานสอง: ต้องมีตัวอย่างประกอบถึงจะเข้าใจ สมมติฐานสาม: จริง ๆ แล้วคำถามนี้เป็นส่วนหนึ่งของงานที่ใหญ่กว่า ควรถามกลับก่อนหนึ่งครั้ง ชั่งน้ำหนักแล้วสมมติฐานแรกกับสองน่าจะครอบคลุมกรณีส่วนใหญ่"},
		{"ตรวจสอบข้อขัดแย้ง", "ลองหาจุดที่สมมติฐานขัดกันเอง — ถ้าตอบสั้นแต่ผู้ใช้ต้องการตัวอย่าง คำตอบจะดูห้วน ถ้าตอบยาวแต่เขาแค่อยากได้คำยืนยัน คำตอบจะดูเยิ่นเย้อ ทางออกคือเปิดด้วยข้อสรุปหนึ่งบรรทัด แล้วค่อยตามด้วยรายละเอียดที่ข้ามได้"},
		{"ร่างคำตอบ", "โครงคำตอบ: บรรทัดแรกสรุปฟันธง ตามด้วยเหตุผลสั้น ๆ สองสามข้อ ปิดด้วยคำถามชวนต่อหนึ่งคำถามถ้าจำเป็น ภาษาต้องเป็นกันเองแต่ไม่หลุดความแม่นยำ หลีกเลี่ยงศัพท์เทคนิคที่ไม่จำเป็น"},
		{"ทบทวนรอบสุดท้าย", "อ่านซ้ำอีกรอบ: ตอบตรงคำถามไหม มีอะไรที่มั่นใจเกินหลักฐานไหม มีทางที่ผู้ใช้จะเข้าใจผิดไหม ถ้าผ่านทั้งสามข้อก็พร้อมตอบ — ท่อนความคิดยาว ๆ ทั้งหมดนี้มีไว้ทดสอบว่า panel แสดงผลลื่น พับเก็บได้ และ scroll ตามได้ถูกต้อง"},
	}
	var b strings.Builder
	for i, s := range sections {
		fmt.Fprintf(&b, "[%d/%d] %s\n%s\n\n", i+1, len(sections), s.head, s.body)
	}
	return strings.TrimSpace(b.String())
}

func clipNoop(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// noopScenario returns a canned reply for UI test keywords, so rendering
// paths (image galleries, tables, broken URLs, ...) can be exercised without
// a live API key: switch provider to noop and type the keyword.
// Deterministic images come from picsum.photos seeds.
func noopScenario(text string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(strings.Fields(text)[0]))
	img := func(seed string, w, h int) string {
		return fmt.Sprintf("![ภาพทดสอบ %s](https://picsum.photos/seed/%s/%d/%d)", seed, seed, w, h)
	}
	switch key {
	case "img1":
		return "รูปเดี่ยวขนาดปกติครับ:\n\n" + img("aetox1", 640, 420) + "\n\nข้อความหลังรูปต้องเว้นระยะสวยงาม", true
	case "img5":
		return "แกลเลอรี 5 รูปติดกัน (ต้องเรียงแถวแล้ว wrap ไม่ใช่ตั้งซ้อนเต็มจอ):\n\n" +
			img("a1", 400, 300) + " " + img("a2", 300, 400) + " " + img("a3", 400, 260) + " " +
			img("a4", 350, 350) + " " + img("a5", 420, 280), true
	case "imgbig":
		return "รูปยักษ์ 4000px (ต้องโดนบีบให้พอดี bubble ไม่ทะลุจอ):\n\n" + img("aetoxbig", 4000, 1400), true
	case "imgbroken":
		return "รูปดี-รูปเสีย-รูปดี (ตัวกลางต้องยุบเป็น alt text ไม่ค้างเป็นซาก):\n\n" +
			img("ok1", 400, 300) + " ![รูปนี้พังแน่นอน](https://aetox.invalid/broken.jpg) " + img("ok2", 400, 300), true
	case "imgmix":
		return "## เทียบมือถือ 3 รุ่น (จำลองคำตอบ research จริง)\n\n" +
			"จากการค้นหา เจอ 3 รุ่นที่น่าสนใจครับ:\n\n" +
			img("phone1", 380, 300) + " " + img("phone2", 380, 300) + " " + img("phone3", 380, 300) + "\n\n" +
			"| รุ่น | ราคา | จุดเด่น |\n|---|---|---|\n| Alpha 12 | 19,900 | กล้อง 200MP |\n| Beta X | 24,500 | แบต 6000mAh |\n| Gamma 5 | 15,900 | คุ้มสุด |\n\n" +
			"- **Alpha 12** เหมาะกับสายถ่ายรูป\n- **Beta X** เหมาะกับสายเกม\n\nอยากดูรีวิวรุ่นไหนบอกได้เลยครับ", true
	case "imghelp", "imgtest":
		return "คีย์เวิร์ดทดสอบ UI รูปภาพ: `img1` เดี่ยว · `img5` แกลเลอรี · `imgbig` รูปยักษ์ · `imgbroken` ลิงก์เสีย · `imgmix` คำตอบ research เต็มรูปแบบ", true
	}
	return "", false
}

// StreamComplete simulates real-model streaming by trickling the noop
// response out word by word, so UI code paths that expect a
// StreamingProvider (typing indicators, incremental render) can be
// exercised without a live API key.
func (p *NoopProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return Response{}, err
	}

	trickle := func(text string, deliver StreamChunkHandler) error {
		for i, word := range strings.Fields(text) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			chunk := word
			if i > 0 {
				chunk = " " + word
			}
			if deliver != nil {
				if err := deliver(chunk); err != nil {
					return err
				}
			}
			time.Sleep(40 * time.Millisecond)
		}
		return nil
	}

	// Reasoning first, then the visible answer — same order as DeepSeek et
	// al., so the live thinking panel gets exercised by aetox-think:test.
	if resp.ReasoningContent != "" {
		if err := trickle(resp.ReasoningContent, onReasoningChunk); err != nil {
			return Response{}, err
		}
	}
	if err := trickle(resp.Text, onChunk); err != nil {
		return Response{}, err
	}

	return resp, nil
}
