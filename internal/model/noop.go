package model

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type NoopProvider struct {
	DefaultModel string
}

func NewNoopProvider(model string) *NoopProvider {
	return &NoopProvider{DefaultModel: model}
}

func (p *NoopProvider) Name() string {
	return "noop"
}

func (p *NoopProvider) SupportsToolCalling() bool {
	return false
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
		model = "noop"
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
			Provider: p.Name(),
			Model:    model,
			ReasoningContent: "กำลังวิเคราะห์คำถาม: \"" + clipNoop(text, 60) + "\" — " +
				"ขั้นแรกแยกประเด็นหลักออกมา จากนั้นพิจารณาบริบทที่เกี่ยวข้อง " +
				"ตรวจสอบสมมติฐานที่เป็นไปได้สองสามทาง แล้วเลือกคำตอบที่ตรงที่สุด " +
				"ข้อความยาว ๆ ท่อนนี้มีไว้ทดสอบ reasoning panel ว่าไหลลื่นและพับเก็บได้ถูกต้อง",
			Text: "[think-test] คำตอบสั้น ๆ หลังคิดเสร็จ: " + clipNoop(text, 80),
		}, nil
	case strings.Contains(modelKey, "markdown"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     noopMarkdownReply(),
		}, nil
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
		Text:     fmt.Sprintf("[noop:%s] %s", model, text),
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
