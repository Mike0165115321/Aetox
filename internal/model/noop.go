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

// GuideTopic is one question the built-in engine can answer about Aetox
// itself. The UI renders the questions as options; clicking one sends it as an
// ordinary message, so the answer streams, persists and scrolls exactly like
// any other reply — one path, not a parallel one (ARCHITECTURE.md §42).
type GuideTopic struct {
	ID       string `json:"id"`
	Question string `json:"question"`
}

// guideAnswers is the whole guide: question and answer, both languages, in one
// place. Matching is done against these exact strings, so the questions the UI
// shows and the questions this matches are the same values by construction.
var guideAnswers = []struct{ id, qTH, qEN, aTH, aEN string }{
	{
		id:  "skills",
		qTH: "สกิล กับ พรอมต์สำเร็จรูป ต่างกันยังไง",
		qEN: "How are skills and prompt presets different?",
		aTH: `ต่างกันที่ **ใครเป็นคนเรียก** ครับ ไม่ใช่ที่เนื้อใน

**พรอมต์สำเร็จรูป** — คุณเป็นคนเรียก พิมพ์ ` + "`/landing`" + ` ในแชท เนื้อพรอมต์จะไปแทนข้อความของคุณ **ก่อน** โมเดลเห็น คุณควบคุมได้ 100% ว่าจะใช้เมื่อไหร่

**สกิล** — โมเดลเป็นคนเรียกเอง มันเห็นรายชื่อสกิลทั้งหมดแล้วตัดสินใจว่าอันไหนเกี่ยวกับงานตรงหน้า คุณไม่ต้องสั่ง

ที่น่าสนใจคือ สกิลแบบไฟล์ ` + "`SKILL.md`" + ` เนื้อในก็คือข้อความล้วนเหมือนกันเป๊ะ — มันไม่ได้เรียกเครื่องมืออะไรเองเลย ต่างกันแค่คนกด

**ผลที่ตามมาจริงๆ มีสองข้อ**
1. พรอมต์สำเร็จรูปกินโทเคนเฉพาะตอนคุณกดใช้ ส่วนสกิลเอาคำอธิบายไปแปะให้โมเดลเห็นทุกเทิร์น ไม่ว่าจะได้ใช้หรือไม่
2. พรอมต์สำเร็จรูปคุณกดเอง รู้ตัวเสมอ ส่วนสกิลโมเดลหยิบไปใช้ได้โดยคุณไม่ได้สั่ง`,
		aEN: `The difference is **who invokes them**, not what is inside.

**Prompt presets** — you invoke them. Type ` + "`/landing`" + ` and the prompt replaces your message **before** the model ever sees it. You decide when, every time.

**Skills** — the model invokes them. It sees the list and decides which one is relevant. You never ask.

The interesting part: a ` + "`SKILL.md`" + ` skill is also just text — it calls no tools of its own. The only difference is who presses the button.

**Two consequences that actually matter**
1. A preset costs tokens only when you use it. A skill puts its description in front of the model every single turn, used or not.
2. You press a preset knowingly. A skill can be pulled in without you asking.`,
	},
	{
		id:  "prompts",
		qTH: "พรอมต์สำเร็จรูปใช้ยังไง",
		qEN: "How do I use prompt presets?",
		aTH: `พิมพ์ ` + "`/`" + ` ในช่องแชท (หรือกดปุ่ม ` + "`/`" + `) แล้วเลือกจากรายการ

พิมพ์ต่อท้ายได้เลย เช่น ` + "`/landing เว็บขายกาแฟคั่วเอง กลุ่มคนทำงาน`" + ` — ข้อความที่พิมพ์ต่อจะไปแทนที่ ` + "`$ARGUMENTS`" + ` ในพรอมต์

ตอนนี้มีให้ 8 ตัว: ` + "`/landing` `/hero` `/pricing` `/waitlist`" + ` ทำเว็บ · ` + "`/review` `/debug` `/explain`" + ` งานโค้ด · ` + "`/clip`" + ` สรุปคลิป

**เพิ่มเองได้** ที่ ตั้งค่า → พรอมต์สำเร็จรูป กดสร้างใหม่ ใส่รูปหน้าปกได้ด้วย และแก้ของที่มีอยู่ได้ทุกตัว (ของเดิมไม่หาย ลบของคุณเมื่อไหร่มันกลับมาเอง)`,
		aEN: `Type ` + "`/`" + ` in the composer (or press the ` + "`/`" + ` button) and pick from the list.

Keep typing after the name: ` + "`/landing a coffee roastery site for office workers`" + ` — everything after the name replaces ` + "`$ARGUMENTS`" + ` in the prompt.

Eight ship with Aetox: ` + "`/landing` `/hero` `/pricing` `/waitlist`" + ` for web work · ` + "`/review` `/debug` `/explain`" + ` for code · ` + "`/clip`" + ` to summarize a recording.

**Add your own** in Settings → Prompt presets. Cover images included, and every shipped preset is editable — the original stays, and deleting yours brings it back.`,
	},
	{
		id:  "connect",
		qTH: "ต่อโมเดลจริงยังไง ทำไมต้องต่อเอง",
		qEN: "How do I connect a real model, and why do I have to?",
		aTH: `Aetox สร้างโดยนักพัฒนาคนเดียว ไม่มีทุนไปซื้อโมเดลมาแจกฟรี — ตรงไปตรงมาแบบนั้นครับ แลกมาด้วยข้อดีคือคุณเลือกเองได้ว่าจะเชื่อใจใคร

**วิธีต่อ** ไปที่ ตั้งค่า → การตั้งค่าโมเดล เลือกผู้ให้บริการ ใส่ API key แล้วกดใช้

**ถ้าไม่อยากให้ข้อมูลออกจากเครื่องเลย** เลือก **Ollama** หรือ **LM Studio** รันโมเดลบนเครื่องคุณเอง ไม่ต้องมี key ไม่มี prompt ไหนออกจากเครื่องแม้แต่ byte เดียว

รองรับ 13 ผู้ให้บริการ สลับได้ตลอด ไม่ผูกมัด`,
		aEN: `Aetox is built by one developer with no funding to give away model access — that is the plain answer. What you get in exchange is choosing who to trust.

**To connect** go to Settings → Model settings, pick a provider, paste an API key, press use.

**To keep everything on your machine**, pick **Ollama** or **LM Studio** and run the model locally. No key, and not a byte of your prompts leaves the machine.

Thirteen providers supported, switchable any time.`,
	},
	{
		id:  "privacy",
		qTH: "ข้อมูลของผมปลอดภัยแค่ไหน",
		qEN: "How safe is my data?",
		aTH: `**ไม่มีเซิร์ฟเวอร์ของเราคั่นกลางเลย** ประวัติแชทและไฟล์โปรเจกต์อยู่ในเครื่องคุณล้วนๆ (SQLite ในเครื่อง) เราไม่เห็นอะไรทั้งนั้น

เวลาคุณต่อ provider คลาวด์ prompt จะวิ่งจากเครื่องคุณไปหาเขาโดยตรง ไม่ผ่านเรา

**เครื่องมือที่แตะเครื่องคุณต้องขออนุมัติ** — รันคำสั่ง เขียนไฟล์ ลบไฟล์ ขึ้นให้กดยืนยันก่อนทุกครั้ง ปรับระดับได้ที่ Ctrl+K → โหมดอนุมัติ

ถอดเสียง อ่านรูป อ่านวิดีโอ ทำในเครื่องทั้งหมด ไฟล์ไม่ได้ถูกอัปโหลดไปไหน`,
		aEN: `**No server of ours sits in the middle.** Chat history and project files stay on your machine (local SQLite). We see none of it.

When you connect a cloud provider, prompts go from your machine straight to them — never through us.

**Anything that touches your machine asks first** — running commands, writing files, deleting files all prompt for confirmation. Adjust the level under Ctrl+K → Approval mode.

Transcription, image reading and video reading all run locally. Those files are not uploaded anywhere.`,
	},
	{
		id:  "tools",
		qTH: "Aetox ทำอะไรได้บ้าง",
		qEN: "What can Aetox actually do?",
		aTH: `มีเครื่องมือในตัว 22 ตัว ที่เด่นคือกลุ่มที่**เติมประสาทสัมผัสให้โมเดล**:

- **อ่านรูป** ` + "`image_ocr`" + ` — โมเดลที่มองไม่เห็นรูปก็อ่านสลิป สกรีนช็อตได้
- **อ่านวิดีโอ** ` + "`video_ocr`" + ` — แตกเฟรมแล้ว OCR ได้ข้อความพร้อมเวลา
- **ฟังเสียง** ` + "`audio_transcribe`" + ` — ถอดเสียงพูดในเครื่อง ไทย+อังกฤษ
- **ลงมือทำในเว็บ** ` + "`browser_*`" + ` — เปิด อ่าน คลิก กรอกฟอร์มจริงได้

บวกกับงานโค้ดครบชุด อ่าน/เขียน/แก้ไฟล์ · git · รันคำสั่ง · ค้นเว็บ · ค้น GitHub

**จุดสำคัญ** เครื่องมือทั้งหมดนี้เสิร์ฟให้ทุกโมเดลเท่ากัน โมเดลเล็กราคาถูกก็ได้ของครบเหมือนกัน — ความสามารถอยู่ที่สถาปัตยกรรม ไม่ใช่ราคาโมเดล`,
		aEN: `22 built-in tools. The standouts are the ones that **give a model senses it does not have**:

- **See images** ` + "`image_ocr`" + ` — a model with no vision still reads receipts and screenshots
- **Read video** ` + "`video_ocr`" + ` — frames sampled and OCR'd, with timestamps
- **Hear** ` + "`audio_transcribe`" + ` — speech to text, locally, Thai and English
- **Act on the web** ` + "`browser_*`" + ` — open, read, click, fill real forms

Plus the full coding set: read/write/edit files · git · shell · web search · GitHub search.

**The point:** every tool is served to every model equally. A cheap small model gets the same kit — the capability lives in the architecture, not the price of the model.`,
	},
	{
		id:  "who",
		qTH: "ใครทำ Aetox และทำไป",
		qEN: "Who makes Aetox, and why?",
		aTH: `นักพัฒนาคนเดียวครับ ไม่มีทีม ไม่มีนักลงทุน ทุกบรรทัดเขียนโดยคนที่ใช้มันทำงานจริงทุกวัน

**ความเชื่อที่อยู่เบื้องหลัง** — หัวใจของ AI agent ไม่ใช่ความรู้ที่อัดอยู่ในโมเดล แต่คือ **สถาปัตยกรรมที่ควบคุมวิธีคิด** โมเดลเก่งแค่ไหนถ้าไม่มีเครื่องมือ ไม่มีขอบเขตความปลอดภัย ไม่มีวิธีจัดการบริบท ก็ทำงานจริงไม่ได้

เราไม่มีทุนเทรนโมเดลเอง เลยเลือกเอาดีทางสถาปัตยกรรมแทน แล้วเปิดให้คุณเสียบโมเดลอะไรก็ได้เข้ามา

ถ้าอยากร่วมพัฒนา หรือลองแล้วอยากบอกว่าตรงไหนห่วย — เปิด issue บน GitHub ได้เลย อ่านทุกข้อความ`,
		aEN: `One developer. No team, no investors. Every line written by someone who uses it for real work daily.

**The belief behind it** — what makes an AI agent useful is not the knowledge packed into the model, it is the **architecture that governs how it works**. However capable the model, without tools, safety boundaries and context management it cannot do real work.

There was no funding to train a model, so the bet went on architecture instead — and on letting you plug in whichever model you like.

Want to help, or just tell us what is bad about it? Open an issue on GitHub. Every message gets read.`,
	},
}

// GuideTopics lists the questions the built-in engine can answer, in the given
// UI language. The UI renders these verbatim and sends the chosen one back as
// an ordinary message.
func GuideTopics(locale string) []GuideTopic {
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
	out := make([]GuideTopic, 0, len(guideAnswers))
	for _, g := range guideAnswers {
		q := g.qTH
		if english {
			q = g.qEN
		}
		out = append(out, GuideTopic{ID: g.id, Question: q})
	}
	return out
}

// guideAnswer matches an incoming message against the guide questions. Both
// languages are matched regardless of the current locale, so a question asked
// before a language switch still resolves.
func (p *NoopProvider) guideAnswer(text string) (string, bool) {
	text = strings.TrimSpace(text)
	for _, g := range guideAnswers {
		if text == g.qTH || text == g.qEN {
			return p.pick(g.aTH, g.aEN), true
		}
	}
	return "", false
}

// english reports whether the UI is in English. Everything else falls back to
// Thai, which is what a fresh install runs in. Every canned string this
// provider produces goes through this — the built-in models are the only part
// of the engine that talks to a user directly (ARCHITECTURE.md §40, §41).
func (p *NoopProvider) english() bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Locale)), "en")
}

// pick is the one-liner every bilingual canned string below uses.
func (p *NoopProvider) pick(th, en string) string {
	if p.english() {
		return en
	}
	return th
}

func (p *NoopProvider) onboardingReply() string {
	return p.pick(noopOnboardingReply, noopOnboardingReplyEN)
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

	// A guide question wins over every model-specific script below: it is the
	// user clicking an option Aetox itself offered, and it must answer the same
	// on whichever built-in model happens to be selected.
	if answer, ok := p.guideAnswer(text); ok {
		return Response{Provider: p.Name(), Model: model, Text: answer}, nil
	}

	// Test models: the picked model name decides the response shape, so each
	// chat-rendering path can be exercised on its own (see provider catalog).
	modelKey := strings.ToLower(model)
	switch {
	case strings.Contains(modelKey, "image"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     p.noopImageReply(text),
		}, nil
	case strings.Contains(modelKey, "think"):
		return Response{
			Provider:         p.Name(),
			Model:            model,
			ReasoningContent: p.noopLongReasoning(text),
			Text: p.pick("[think-test] คำตอบสั้น ๆ หลังคิดเสร็จ: ", "[think-test] short answer after thinking: ") +
				clipNoop(text, 80),
		}, nil
	case strings.Contains(modelKey, "markdown"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     p.noopMarkdownReply(),
		}, nil
	case strings.Contains(modelKey, "tools"):
		return p.noopToolsReply(model, req), nil
	}

	if scripted, ok := p.noopScenario(text); ok {
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
func (p *NoopProvider) noopImageReply(text string) string {
	if scripted, ok := p.noopScenario(text); ok {
		return scripted
	}
	scripted, _ := p.noopScenario("imgmix")
	return scripted
}

func (p *NoopProvider) noopMarkdownReply() string {
	if p.english() {
		return "## Full markdown test\n\n" +
			"A normal paragraph with **bold**, *italic*, `inline code` and a [link](https://example.com)\n\n" +
			"```go\nfunc main() {\n\tfmt.Println(\"code block\")\n}\n```\n\n" +
			"| Column | Value |\n|---|---|\n| One | 111 |\n| Two | 222 |\n\n" +
			"1. Ordered list\n2. Second item\n\n- Bullet list\n- Second item\n\n> A quotation (blockquote)\n\n---\n\nEnd of the test set."
	}
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
			ID:       id,
			Type:     "function",
			Function: FunctionCall{Name: name, Arguments: args},
		}}}
	}
	switch {
	case todoCalls == 0:
		return call("noop_todo_1", "todo_write", p.pick(
			`{"todos":[{"content":"วางแผนชุดทดสอบ UI","status":"completed"},{"content":"แสดง checklist ระหว่างทำงาน","status":"in_progress"},{"content":"ถามผู้ใช้ด้วย ask_user","status":"pending"},{"content":"สรุปผลการทดสอบ","status":"pending"}]}`,
			`{"todos":[{"content":"Plan the UI test set","status":"completed"},{"content":"Show the checklist while working","status":"in_progress"},{"content":"Ask the user via ask_user","status":"pending"},{"content":"Summarize the results","status":"pending"}]}`))
	case askCalls == 0:
		return call("noop_ask_1", "ask_user", p.pick(
			`{"question":"ทดสอบ ask_user: อยากให้ตอบกลับด้วยโทนไหนครับ?","options":["สั้น กระชับ","ละเอียด ยกตัวอย่าง","ขำๆ มีอีโมจิ","ทางการ"]}`,
			`{"question":"ask_user test: which tone should the reply use?","options":["Short and tight","Detailed, with examples","Playful, with emoji","Formal"]}`))
	case todoCalls == 1:
		return call("noop_todo_2", "todo_write", p.pick(
			`{"todos":[{"content":"วางแผนชุดทดสอบ UI","status":"completed"},{"content":"แสดง checklist ระหว่างทำงาน","status":"completed"},{"content":"ถามผู้ใช้ด้วย ask_user","status":"completed"},{"content":"สรุปผลการทดสอบ","status":"completed"}]}`,
			`{"todos":[{"content":"Plan the UI test set","status":"completed"},{"content":"Show the checklist while working","status":"completed"},{"content":"Ask the user via ask_user","status":"completed"},{"content":"Summarize the results","status":"completed"}]}`))
	default:
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text: p.pick(
				"✅ จบชุดทดสอบ tools UI ครับ — todo panel, ask_user cards และ tool timeline ทำงานครบ\n\nผลจาก ask_user: ",
				"✅ Tools UI test set complete — todo panel, ask_user cards and the tool timeline all worked.\n\nask_user returned: ") + lastAnswer,
		}
	}
}

// noopLongReasoning produces a multi-paragraph thinking stream (~2 minutes of
// word-by-word trickle) so the live reasoning panel, its unbounded height, the
// pinned auto-scroll, and the collapsed "done thinking" toggle all get a real
// workout without an API key.
func (p *NoopProvider) noopLongReasoning(text string) string {
	if p.english() {
		return renderReasoning([]struct{ head, body string }{
			{"Reading the question", "The user asked \"" + clipNoop(text, 60) + "\" — first, separate whether this is a question of fact, of opinion, or an instruction to act, because the shape of the answer differs completely. Misread it here and everything downstream is wrong."},
			{"Breaking it down", "Three axes: (1) what was literally said (2) what they probably want but did not say (3) the surrounding constraints — time, earlier context, the right format to answer in. The second is the most important and the easiest to miss."},
			{"Hypotheses", "First: a short direct answer is enough. Second: it needs an example to land. Third: this is part of a larger task and one clarifying question should come first. Weighing them, the first two cover most cases."},
			{"Looking for contradictions", "Where do the hypotheses fight? Answer short when they wanted an example and it reads curt; answer long when they wanted confirmation and it reads bloated. The way out is a one-line conclusion first, then detail that can be skipped."},
			{"Drafting", "Structure: a decisive first line, two or three short reasons, and one follow-up question only if it is genuinely needed. Friendly but precise, no jargon that is not earning its place."},
			{"Final pass", "Read it again: does it answer what was asked, is anything stated with more confidence than the evidence supports, is there a way to misread it? All three clear means it is ready — and this whole long stream exists to prove the panel renders smoothly, collapses, and keeps scroll in the right place."},
		})
	}
	sections := []struct{ head, body string }{
		{"ตีโจทย์", "ผู้ใช้ถามว่า \"" + clipNoop(text, 60) + "\" — ก่อนอื่นต้องแยกให้ออกว่านี่คือคำถามเชิงข้อเท็จจริง เชิงความเห็น หรือเป็นคำสั่งให้ลงมือทำ เพราะโครงคำตอบต่างกันมาก ถ้าตีความผิดตั้งแต่ต้น ที่เหลือจะเพี้ยนหมด"},
		{"แตกประเด็น", "ลองแตกออกเป็นสามแกน: (1) สิ่งที่ผู้ใช้พูดตรง ๆ (2) สิ่งที่น่าจะอยากได้จริง ๆ แต่ไม่ได้พูด (3) ข้อจำกัดแวดล้อม เช่น เวลา บริบทก่อนหน้าในบทสนทนา และรูปแบบคำตอบที่เหมาะ ประเด็นที่สองสำคัญสุดและพลาดง่ายสุด"},
		{"ตั้งสมมติฐาน", "สมมติฐานแรก: ตอบสั้นตรงประเด็นพอ สมมติฐานสอง: ต้องมีตัวอย่างประกอบถึงจะเข้าใจ สมมติฐานสาม: จริง ๆ แล้วคำถามนี้เป็นส่วนหนึ่งของงานที่ใหญ่กว่า ควรถามกลับก่อนหนึ่งครั้ง ชั่งน้ำหนักแล้วสมมติฐานแรกกับสองน่าจะครอบคลุมกรณีส่วนใหญ่"},
		{"ตรวจสอบข้อขัดแย้ง", "ลองหาจุดที่สมมติฐานขัดกันเอง — ถ้าตอบสั้นแต่ผู้ใช้ต้องการตัวอย่าง คำตอบจะดูห้วน ถ้าตอบยาวแต่เขาแค่อยากได้คำยืนยัน คำตอบจะดูเยิ่นเย้อ ทางออกคือเปิดด้วยข้อสรุปหนึ่งบรรทัด แล้วค่อยตามด้วยรายละเอียดที่ข้ามได้"},
		{"ร่างคำตอบ", "โครงคำตอบ: บรรทัดแรกสรุปฟันธง ตามด้วยเหตุผลสั้น ๆ สองสามข้อ ปิดด้วยคำถามชวนต่อหนึ่งคำถามถ้าจำเป็น ภาษาต้องเป็นกันเองแต่ไม่หลุดความแม่นยำ หลีกเลี่ยงศัพท์เทคนิคที่ไม่จำเป็น"},
		{"ทบทวนรอบสุดท้าย", "อ่านซ้ำอีกรอบ: ตอบตรงคำถามไหม มีอะไรที่มั่นใจเกินหลักฐานไหม มีทางที่ผู้ใช้จะเข้าใจผิดไหม ถ้าผ่านทั้งสามข้อก็พร้อมตอบ — ท่อนความคิดยาว ๆ ทั้งหมดนี้มีไว้ทดสอบว่า panel แสดงผลลื่น พับเก็บได้ และ scroll ตามได้ถูกต้อง"},
	}
	return renderReasoning(sections)
}

func renderReasoning(sections []struct{ head, body string }) string {
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
func (p *NoopProvider) noopScenario(text string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(strings.Fields(text)[0]))
	alt := p.pick("ภาพทดสอบ", "test image")
	img := func(seed string, w, h int) string {
		return fmt.Sprintf("![%s %s](https://picsum.photos/seed/%s/%d/%d)", alt, seed, seed, w, h)
	}
	switch key {
	case "img1":
		return p.pick("รูปเดี่ยวขนาดปกติครับ:", "A single image at normal size:") + "\n\n" + img("aetox1", 640, 420) +
			"\n\n" + p.pick("ข้อความหลังรูปต้องเว้นระยะสวยงาม", "Text after the image must keep clean spacing."), true
	case "img5":
		return p.pick(
			"แกลเลอรี 5 รูปติดกัน (ต้องเรียงแถวแล้ว wrap ไม่ใช่ตั้งซ้อนเต็มจอ):",
			"Five images in a row (they must flow and wrap, not stack full-width):") + "\n\n" +
			img("a1", 400, 300) + " " + img("a2", 300, 400) + " " + img("a3", 400, 260) + " " +
			img("a4", 350, 350) + " " + img("a5", 420, 280), true
	case "imgbig":
		return p.pick(
			"รูปยักษ์ 4000px (ต้องโดนบีบให้พอดี bubble ไม่ทะลุจอ):",
			"A 4000px monster (must be constrained to the bubble, not blow past the screen):") + "\n\n" +
			img("aetoxbig", 4000, 1400), true
	case "imgbroken":
		return p.pick(
			"รูปดี-รูปเสีย-รูปดี (ตัวกลางต้องยุบเป็น alt text ไม่ค้างเป็นซาก):",
			"Good-broken-good (the middle one must collapse to alt text, not leave a carcass):") + "\n\n" +
			img("ok1", 400, 300) + " ![" + p.pick("รูปนี้พังแน่นอน", "this one is definitely broken") +
			"](https://aetox.invalid/broken.jpg) " + img("ok2", 400, 300), true
	case "imgmix":
		if p.english() {
			return "## Three phones compared (a simulated research answer)\n\n" +
				"The search turned up three worth looking at:\n\n" +
				img("phone1", 380, 300) + " " + img("phone2", 380, 300) + " " + img("phone3", 380, 300) + "\n\n" +
				"| Model | Price | Standout |\n|---|---|---|\n| Alpha 12 | 19,900 | 200MP camera |\n| Beta X | 24,500 | 6000mAh battery |\n| Gamma 5 | 15,900 | best value |\n\n" +
				"- **Alpha 12** for photography\n- **Beta X** for gaming\n\nSay which one you want reviewed.", true
		}
		return "## เทียบมือถือ 3 รุ่น (จำลองคำตอบ research จริง)\n\n" +
			"จากการค้นหา เจอ 3 รุ่นที่น่าสนใจครับ:\n\n" +
			img("phone1", 380, 300) + " " + img("phone2", 380, 300) + " " + img("phone3", 380, 300) + "\n\n" +
			"| รุ่น | ราคา | จุดเด่น |\n|---|---|---|\n| Alpha 12 | 19,900 | กล้อง 200MP |\n| Beta X | 24,500 | แบต 6000mAh |\n| Gamma 5 | 15,900 | คุ้มสุด |\n\n" +
			"- **Alpha 12** เหมาะกับสายถ่ายรูป\n- **Beta X** เหมาะกับสายเกม\n\nอยากดูรีวิวรุ่นไหนบอกได้เลยครับ", true
	case "imghelp", "imgtest":
		return p.pick(
			"คีย์เวิร์ดทดสอบ UI รูปภาพ: `img1` เดี่ยว · `img5` แกลเลอรี · `imgbig` รูปยักษ์ · `imgbroken` ลิงก์เสีย · `imgmix` คำตอบ research เต็มรูปแบบ",
			"Image UI test keywords: `img1` single · `img5` gallery · `imgbig` oversized · `imgbroken` dead link · `imgmix` a full research-style answer"), true
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
