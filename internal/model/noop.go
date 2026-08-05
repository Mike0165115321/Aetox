package model

import (
	"context"
	"fmt"
	"strconv"
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
		qTH: "สกิล กับ ชุดคำสั่ง ต่างกันยังไง",
		qEN: "How are skills and prompt presets different?",
		aTH: `ต่างกันที่ **ใครเป็นคนเรียก** ครับ ไม่ใช่ที่เนื้อใน

**ชุดคำสั่ง** — คุณเป็นคนเรียก พิมพ์ ` + "`/landing`" + ` ในแชท เนื้อคำสั่งจะไปแทนข้อความของคุณ **ก่อน** โมเดลเห็น คุณควบคุมได้ 100% ว่าจะใช้เมื่อไหร่

**สกิล** — โมเดลเป็นคนเรียกเอง มันเห็นรายชื่อสกิลทั้งหมดแล้วตัดสินใจว่าอันไหนเกี่ยวกับงานตรงหน้า คุณไม่ต้องสั่ง

ที่น่าสนใจคือ สกิลแบบไฟล์ ` + "`SKILL.md`" + ` เนื้อในก็คือข้อความล้วนเหมือนกันเป๊ะ — มันไม่ได้เรียกเครื่องมืออะไรเองเลย ต่างกันแค่คนกด

**ผลที่ตามมาจริงๆ มีสองข้อ**
1. ชุดคำสั่งกินโทเคนเฉพาะตอนคุณกดใช้ ส่วนสกิลเอาคำอธิบายไปแปะให้โมเดลเห็นทุกเทิร์น ไม่ว่าจะได้ใช้หรือไม่
2. ชุดคำสั่งคุณกดเอง รู้ตัวเสมอ ส่วนสกิลโมเดลหยิบไปใช้ได้โดยคุณไม่ได้สั่ง`,
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
		qTH: "ชุดคำสั่งใช้ยังไง",
		qEN: "How do I use prompt presets?",
		aTH: `พิมพ์ ` + "`/`" + ` ในช่องแชท (หรือกดปุ่ม ` + "`/`" + `) แล้วเลือกจากรายการ

พิมพ์ต่อท้ายได้เลย เช่น ` + "`/landing เว็บขายกาแฟคั่วเอง กลุ่มคนทำงาน`" + ` — ข้อความที่พิมพ์ต่อจะไปแทนที่ ` + "`$ARGUMENTS`" + ` ในชุดคำสั่ง

ตอนนี้มีให้ 8 ตัว: ` + "`/landing` `/hero` `/pricing` `/waitlist`" + ` ทำเว็บ · ` + "`/review` `/debug` `/explain`" + ` งานโค้ด · ` + "`/clip`" + ` สรุปคลิป

**เพิ่มเองได้** ที่ ตั้งค่า → ชุดคำสั่ง กดสร้างใหม่ ใส่รูปหน้าปกได้ด้วย และแก้ของที่มีอยู่ได้ทุกตัว (ของเดิมไม่หาย ลบของคุณเมื่อไหร่มันกลับมาเอง)`,
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
	// (todo panel, ask_user cards, tool timeline) is exercisable without a key;
	// aetox-subagent:test does the same for delegation, and needs it on both
	// sides — a delegate runs on whatever model the session is using.
	name := strings.ToLower(p.DefaultModel)
	return strings.Contains(name, "tools") || strings.Contains(name, "subagent")
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
	// "image" and "markdown" are the names this model had when it was two.
	// They were never two benches: images arrive as markdown, and both were one
	// canned rich reply through one renderer. Kept as aliases rather than
	// retired, for the reason the provider catalog keeps its own — a preference
	// file saved before the merge still names one of them, and a model id that
	// stops resolving is a chat that stops working.
	case strings.Contains(modelKey, "render"),
		strings.Contains(modelKey, "image"),
		strings.Contains(modelKey, "markdown"):
		return Response{
			Provider: p.Name(),
			Model:    model,
			Text:     p.noopRenderReply(text),
		}, nil
	case strings.Contains(modelKey, "think"):
		return Response{
			Provider:         p.Name(),
			Model:            model,
			ReasoningContent: p.noopLongReasoning(text),
			Text: p.pick("[think-test] คำตอบสั้น ๆ หลังคิดเสร็จ: ", "[think-test] short answer after thinking: ") +
				clipNoop(text, 80),
		}, nil
	case strings.Contains(modelKey, "subagent"):
		return p.noopSubagentReply(model, req), nil
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

// noopRenderReply is the whole chat renderer in one answer: markdown structure
// (headings, code, a table, both list kinds, a quote, a rule) followed by a
// research-style reply with an image gallery in it.
//
// It was two models, `aetox-image:test` and `aetox-markdown:test`, and the
// split did not survive looking at it: an image *is* markdown, both were a
// single canned reply, and the only thing either proved was that one renderer
// draws what it is given. Checking a table and checking a gallery in one glance
// is strictly more than checking them in two.
//
// Isolation did not go anywhere — the img* keywords still pick one case out
// (single, wrap, oversized, broken), and they always worked on every built-in
// model rather than only on the image one.
func (p *NoopProvider) noopRenderReply(text string) string {
	if scripted, ok := p.noopScenario(text); ok {
		return scripted
	}
	gallery, _ := p.noopScenario("imgmix")
	return p.noopMarkdownReply() + "\n\n---\n\n" + gallery
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
	// Delegating wins over proposing, and the order is load-bearing rather than
	// arbitrary. A brief saying `subagent memory` means the *delegate* is the
	// one that should remember something; a parent that took the memory branch
	// first would file the proposal against the main agent and the bench would
	// show a clean run of the exact failure it exists to catch. (It did, once —
	// which is why this is two ordered checks and not one combined condition.)
	//
	// Whoever holds `task` is a parent: delegates are force-denied all three
	// halves (internal/subagent), so this cannot fire on a child.
	if offersTool(req, "task") && briefMentions(req, "subagent") {
		return p.noopDelegationReply(model, req)
	}
	// Opt-in per brief, like askmain and toolchain below: one round that
	// proposes something to remember, so the approval queue and everything
	// behind it can be exercised with no API key. Not folded into the default
	// script, because that one ends in a blocking ask_user and this has to stay
	// a single round a test can drive to completion.
	if briefMentions(req, "memory") && offersTool(req, "memory") {
		return p.noopMemoryReply(model, req)
	}
	// Script only tools the caller actually offered. A sub-agent runs on a
	// filtered registry — `todo_write` and `ask_user` are force-denied to every
	// delegate (internal/subagent) — and a model that calls a tool it was never
	// given just fails at dispatch, which tests nothing. Real models behave the
	// same way, so this is fidelity, not a special case.
	//
	// A request that declares no tools at all is the caller not saying (some
	// tests, and any hand-built Request): keep the historical UI script rather
	// than guessing it is a delegate.
	if len(req.Tools) > 0 && (!offersTool(req, "todo_write") || !offersTool(req, "ask_user")) {
		return p.noopDelegateToolsReply(model, req)
	}
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

// noopMemoryReply drives the learning floor (ARCHITECTURE.md §82) end to end
// without a key: propose one durable fact, then report what came back.
//
// The reported text matters as much as the call. `memory` answers "queued for
// the user to approve", never "saved", and a bench that hid that answer would
// let the tool start claiming the write and nobody would notice — which is the
// one failure that turns "nothing takes effect without you" into a lie.
//
// Stateless like its siblings: which round we are in is read off the transcript.
func (p *NoopProvider) noopMemoryReply(model string, req Request) Response {
	for _, m := range req.Messages {
		if m.Role == RoleTool && strings.EqualFold(m.Name, "memory") {
			return Response{Provider: p.Name(), Model: model, Text: p.pick(
				"[tools-test] เสนอสิ่งที่อยากจำไปแล้ว ระบบตอบกลับว่า: ",
				"[tools-test] proposed something to remember. It answered: ") +
				clipNoop(strings.TrimSpace(m.Content), 300)}
		}
	}
	return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
		ID:   "noop_memory_1",
		Type: "function",
		Function: FunctionCall{Name: "memory", Arguments: p.pick(
			`{"text":"เครื่องนี้ไม่มี Excel ติดตั้ง ต้องส่งไฟล์ .xlsx กลับเป็นไฟล์เสมอ","why":"[tools-test] ชุดทดสอบการเรียนรู้"}`,
			`{"text":"This machine has no Excel installed, so .xlsx work has to come back as a file","why":"[tools-test] the learning test set"}`)},
	}}}
}

// briefMentions reports whether the user side of the transcript contains a
// keyword. A delegate's brief is a user message, so this is how a test asks the
// scripted delegate for a specific behaviour without a second provider.
func briefMentions(req Request, keyword string) bool {
	for _, m := range req.Messages {
		if m.Role == RoleUser && strings.Contains(strings.ToLower(m.Content), keyword) {
			return true
		}
	}
	return false
}

// noopSubagentReply is the delegation UI on a bench: switch to
// aetox-subagent:test, send anything, and the whole §44 path runs with no API
// key — a delegate is started, it gets stuck and asks, the answer goes back, and
// the finished work is collected. What that exercises on screen is the part no
// unit test can look at: the nested tool timeline, a delegate's rows appearing
// under its `task` row, and the question sitting in the transcript until it is
// answered.
//
// Both sides of the delegation run on this same model — a delegate inherits the
// session's provider — so this function is the parent script and hands the child
// to noopDelegateToolsReply, told apart by which tools the request offers. A
// delegate never sees `task`, structurally (internal/subagent, depth 1).
//
// Stateless like its siblings: the round is read off the transcript, so it
// survives a re-bootstrap and cannot drift from what actually happened.
func (p *NoopProvider) noopSubagentReply(model string, req Request) Response {
	if !offersTool(req, "task") {
		return p.noopSubagentDelegateReply(model, req)
	}

	started, collected, answered := 0, 0, 0
	for _, m := range req.Messages {
		if m.Role != RoleTool {
			continue
		}
		switch strings.ToLower(m.Name) {
		case "task":
			started++
		case "task_result":
			collected++
		case "task_answer":
			answered++
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
	case started == 0:
		// "askmain" in the brief is what makes the delegate stop and ask — the
		// same opt-in keyword the offline tests use, so the bench and the suite
		// exercise one script rather than two.
		// `general`, not `explore`: the bench is there to be watched, and a
		// read-only delegate can only ever show a search. This one lists, writes
		// a real file, reads it back and greps it — the shape of an actual
		// delegated job, and an artifact on disk at the end of it.
		return call("noop_task_1", "task", p.pick(
			`{"description":"ทดสอบซับเอเจน","prompt":"askmain: ดูว่าในโฟลเดอร์นี้มีไฟล์อะไรบ้าง เขียนไฟล์สรุปไว้หนึ่งไฟล์ อ่านกลับมายืนยัน แล้วรายงานผล","agent":"general"}`,
			`{"description":"sub-agent test","prompt":"askmain: list what is in this folder, write a summary file, read it back to confirm, then report","agent":"general"}`))
	case collected == 0:
		return call("noop_task_collect_1", "task_result", `{"task_id":"`+firstTaskID(req)+`"}`)
	case answered == 0:
		// The delegate is parked on its question; this is the round that proves
		// the answer travels back into a run that is still alive.
		return call("noop_task_answer_1", "task_answer", p.pick(
			`{"task_id":"`+firstTaskID(req)+`","answer":"ลุยเลย ลิสต์ทั้งโฟลเดอร์ได้"}`,
			`{"task_id":"`+firstTaskID(req)+`","answer":"go ahead, list the whole folder"}`))
	case collected == 1:
		return call("noop_task_collect_2", "task_result", `{"task_id":"`+firstTaskID(req)+`"}`)
	}

	last := ""
	for _, m := range req.Messages {
		if m.Role == RoleTool && strings.EqualFold(m.Name, "task_result") {
			last = strings.TrimSpace(m.Content)
		}
	}
	return Response{Provider: p.Name(), Model: model, Text: p.pick(
		"✅ จบชุดทดสอบซับเอเจนครับ — สั่งงาน → มันติดแล้วถามกลับ → ตอบไป → มันทำต่อจนจบ แล้วเก็บผลได้ครบ\n\nผลจากซับเอเจน: ",
		"✅ Sub-agent test set complete — delegated, it got stuck and asked, the answer went back, it finished, and the work was collected.\n\nWhat the sub-agent returned: ") + clipNoop(last, 600)}
}

// noopSubagentDelegateReply is the bench's delegate: a job with several steps in
// it, because one tool call shows nothing about how a delegation reads on
// screen. It asks first, then lists, writes a summary file, reads it back and
// greps it — five rows under one sub-agent block, ending in a real artifact.
//
// Every step is conditional on the tool actually being offered, so the same
// script degrades gracefully on a profile with a narrower allowlist rather than
// calling something it was never handed. Stateless like its siblings: which step
// we are on is read off the transcript.
func (p *NoopProvider) noopSubagentDelegateReply(model string, req Request) Response {
	const demoFile = "subagent-demo.md"
	ran := map[string]string{}
	for _, m := range req.Messages {
		if m.Role == RoleTool {
			ran[strings.ToLower(strings.TrimSpace(m.Name))] = strings.TrimSpace(m.Content)
		}
	}
	did := func(name string) bool { _, ok := ran[name]; return ok }
	step := func(id, name, args string) Response {
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: args},
		}}}
	}
	body := p.pick(
		"# สรุปโฟลเดอร์\\n\\nไฟล์นี้เขียนโดยซับเอเจนระหว่างชุดทดสอบ\\n\\n- ตรวจแล้ว: OK\\n",
		"# Folder summary\\n\\nWritten by a sub-agent during the test set.\\n\\n- checked: OK\\n")

	switch {
	// A chair's whole job is one deliverable, so its script is one call: the
	// writer its desk carries, then the report. It cannot walk the file-tool
	// sequence below — the office manifest has no `edit`, no `grep`, and the
	// chair asked for none of them — and a script that tried would only prove
	// that calling a tool you were not handed goes nowhere.
	case briefMentions(req, "office") && offersTool(req, "doc_write") && !did("doc_write"):
		return step("noop_sub_doc", "doc_write", p.pick(
			`{"path":"office-demo.docx","blocks":[{"type":"heading","text":"สรุปงานจากออฟฟิศ"},{"type":"paragraph","text":"เขียนโดยเก้าอี้ในออฟฟิศระหว่างชุดทดสอบ"}]}`,
			`{"path":"office-demo.docx","blocks":[{"type":"heading","text":"Office job summary"},{"type":"paragraph","text":"Written by a chair in the office during the test set."}]}`))
	case !did("ask_main") && offersTool(req, "ask_main") && briefMentions(req, "askmain"):
		return step("noop_sub_ask", "ask_main", p.pick(
			`{"question":"[tools-test] ให้ลิสต์ทั้งโฟลเดอร์แล้วเขียนไฟล์สรุปเลยไหม หรือหยุดแค่นี้?"}`,
			`{"question":"[tools-test] list the whole folder and write the summary file, or stop here?"}`))
	case !did("list") && offersTool(req, "list"):
		return step("noop_sub_list", "list", `{"path":"."}`)
	case !did("write") && offersTool(req, "write"):
		return step("noop_sub_write", "write", `{"path":"`+demoFile+`","content":"`+body+`"}`)
	case !did("read") && offersTool(req, "read"):
		return step("noop_sub_read", "read", `{"path":"`+demoFile+`"}`)
	case !did("grep") && offersTool(req, "grep"):
		return step("noop_sub_grep", "grep", `{"pattern":"checked|ตรวจแล้ว","path":"`+demoFile+`"}`)
	}

	var b strings.Builder
	b.WriteString(p.pick("[tools-test] ซับเอเจนทำงานเสร็จแล้ว:\n", "[tools-test] the sub-agent is done:\n"))
	for _, s := range []struct{ tool, th, en string }{
		{"doc_write", "เขียนเอกสารส่งกลับ", "wrote the document"},
		{"list", "ดูไฟล์ในโฟลเดอร์", "listed the folder"},
		{"write", "เขียนไฟล์ " + demoFile, "wrote " + demoFile},
		{"read", "อ่านกลับมายืนยัน", "read it back to confirm"},
		{"grep", "ค้นข้อความในไฟล์", "grepped it"},
	} {
		if out, ok := ran[s.tool]; ok {
			fmt.Fprintf(&b, "- %s: %s\n", p.pick(s.th, s.en), clipNoop(out, 160))
		}
	}
	if answer, ok := ran["ask_main"]; ok {
		// Echoing the answer proves the delegate resumed the same run rather
		// than being restarted: a fresh one would not remember having asked.
		b.WriteString(p.pick("คำตอบจากเมนเอเจน: ", "main agent answered: ") + clipNoop(answer, 200) + "\n")
	}
	return Response{Provider: p.Name(), Model: model, Text: strings.TrimRight(b.String(), "\n")}
}

// firstTaskID digs the handle out of whatever `task` reported. The id travels to
// the model inside a tool result, so the script has to read it back the same way
// a real model would rather than being handed it — which is also what makes this
// break loudly if the handle ever stops being named in that result.
func firstTaskID(req Request) string {
	for _, m := range req.Messages {
		if m.Role != RoleTool || !strings.EqualFold(m.Name, "task") {
			continue
		}
		for _, field := range strings.FieldsFunc(m.Content, func(r rune) bool {
			return r != '_' && !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
		}) {
			if rest, ok := strings.CutPrefix(field, "task_"); ok && rest != "" {
				if _, err := strconv.Atoi(rest); err == nil {
					return field
				}
			}
		}
	}
	return "task_1" // nothing to read: let the tool report the real failure
}

// offersTool reports whether this request actually put the named tool on the
// table. The scripted provider consults it for the same reason a real model
// does: calling something you were not handed goes nowhere.
func offersTool(req Request, name string) bool {
	for _, t := range req.Tools {
		if strings.EqualFold(strings.TrimSpace(t.Function.Name), name) {
			return true
		}
	}
	return false
}

// noopDelegationReply is the parent half of §44 as a script: start a delegate,
// go and collect it, report. It exists because the CLI has no todo_write/ask_user
// — so its main agent lands on the delegate script below and would never touch
// `task`, leaving delegation the one path with no keyless way to exercise it.
//
// Two rounds and no more, deliberately: this proves the wiring the parent owns
// (a handle comes back without blocking, task_result redeems it). What the
// delegate does inside is the script below, driven by the same provider.
//
// The brief picks the profile — "general" if it says so, otherwise explore, which
// is what `task` itself defaults to.
func (p *NoopProvider) noopDelegationReply(model string, req Request) Response {
	started, collected := "", ""
	for _, m := range req.Messages {
		if m.Role != RoleTool {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(m.Name)) {
		case "task":
			if id := taskIDIn(m.Content); id != "" {
				started = id
			}
		case "task_result":
			collected = strings.TrimSpace(m.Content)
		}
	}

	profile := "explore"
	switch {
	case briefMentions(req, "general"):
		profile = "general"
	// "office" hands the job to a chair instead of a delegate (COMPANY.md §4):
	// a whole deliverable, briefed once, coming back as a file. It is the one
	// script where the sub-agent runs on a *different* desk's manifest than the
	// caller's, which is the §84 carve-out and has no other keyless surface.
	case briefMentions(req, "office") && offersTool(req, "task"):
		profile = "doc"
	}
	call := func(id, name, args string) Response {
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID:       id,
			Type:     "function",
			Function: FunctionCall{Name: name, Arguments: args},
		}}}
	}

	// Keywords the delegate's own script reads have to survive into its brief, or
	// they can only be used from a test that writes the brief by hand — the CLI
	// could never reach them, which is the surface this script exists to serve.
	passthrough := ""
	for _, keyword := range []string{"toolchain", "askmain", "memory", "office"} {
		if briefMentions(req, keyword) {
			passthrough += keyword + ": "
		}
	}

	switch {
	case started == "":
		return call("noop_task_1", "task", fmt.Sprintf(
			`{"description":"list the sandbox root","prompt":"%sList what is in the sandbox root and report the paths you found. Nothing else.","agent":%q}`,
			passthrough, profile))
	case collected == "":
		return call("noop_task_collect_1", "task_result", fmt.Sprintf(`{"task_id":%q}`, started))
	}
	return Response{Provider: p.Name(), Model: model, Text: p.pick(
		"[tools-test] ส่งงานให้ซับเอเจน "+profile+" แล้วเก็บผลกลับมาได้ ผลที่ได้:\n",
		"[tools-test] delegated to sub-agent "+profile+" and collected it. Result:\n") + clipNoop(collected, 600)}
}

// taskIDIn digs the handle out of what `task` returned. It reads the id rather
// than assuming "task_1" so a script that starts two delegates still collects the
// right one. task_result is skipped explicitly — it shares the prefix.
func taskIDIn(content string) string {
	for _, field := range strings.Fields(content) {
		id := strings.Trim(field, `"'.,;:—`)
		if !strings.HasPrefix(id, "task_") || id == "task_result" {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(id, "task_")); err == nil {
			return id
		}
	}
	return ""
}

// toolchainProbes is the sequence a brief containing "toolchain" walks — one
// read-only tool per round, each with arguments that work in any sandbox,
// including an empty one. Every other script here stops after a single call,
// which proves a delegate *can* call a tool but not that its loop survives
// several of them with results accumulating in between.
var toolchainProbes = []struct{ name, args string }{
	{"list", `{"path":"."}`},
	{"glob", `{"pattern":"*"}`},
	{"grep", `{"pattern":"e","path":"."}`},
}

// noopToolchainReply runs each probe it was actually handed, one per round, then
// reports what every one of them returned. Stateless like its siblings: which
// round we are in is read off the transcript, never counted.
func (p *NoopProvider) noopToolchainReply(model string, req Request) Response {
	ran := map[string]string{}
	for _, m := range req.Messages {
		if m.Role == RoleTool {
			ran[strings.ToLower(strings.TrimSpace(m.Name))] = strings.TrimSpace(m.Content)
		}
	}
	for _, probe := range toolchainProbes {
		if _, done := ran[probe.name]; done || !offersTool(req, probe.name) {
			continue
		}
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID:       "noop_chain_" + probe.name,
			Type:     "function",
			Function: FunctionCall{Name: probe.name, Arguments: probe.args},
		}}}
	}
	if len(ran) == 0 {
		return Response{Provider: p.Name(), Model: model, Text: p.pick(
			"[tools-test] ไม่ได้รับเครื่องมืออ่านข้อมูลเลย จึงไม่มีอะไรให้รายงาน",
			"[tools-test] I was handed no read-only tool, so there is nothing to report")}
	}
	// A digest, not the raw envelopes — which is what a delegate is for, and what
	// makes the report checkable at both ends: a loop that quietly stopped after
	// the first probe cannot fake these lines, and a raw tool envelope appearing
	// in what the parent receives can only mean the transcript leaked (§44.6).
	var b strings.Builder
	b.WriteString(p.pick("[tools-test] เรียกครบทุกตัวแล้ว:", "[tools-test] ran the whole chain:"))
	for _, probe := range toolchainProbes {
		if out, done := ran[probe.name]; done {
			fmt.Fprintf(&b, "\n- %s → %d bytes", probe.name, len(out))
		}
	}
	return Response{Provider: p.Name(), Model: model, Text: b.String()}
}

// noopDelegateToolsReply is the tool-loop script for a caller running on a
// trimmed tool set: a sub-agent (internal/subagent), or anyone else — the engine
// registry alone has no ask_user/todo_write either, since those are desktop-side
// skills. Two rounds, mirroring what a delegate does: run one read-only tool,
// then report the result and stop. It is `list` rather than `grep` because
// listing needs no pattern to match, so the round is meaningful in any sandbox
// including an empty one.
//
// Stateless like its sibling: which round we are in is read off the transcript,
// so it survives a re-bootstrap mid-run.
func (p *NoopProvider) noopDelegateToolsReply(model string, req Request) Response {
	// Opt-in per brief, like askmain: a delegate that calls three tools instead of
	// one, so the child's *loop* is exercised and not just its first call.
	if briefMentions(req, "toolchain") {
		return p.noopToolchainReply(model, req)
	}
	// The same bench from the other side of the scope line. A delegate holds a
	// `memory` bound to its own profile (internal/subagent), and the claim worth
	// checking by hand is which file the proposal lands in — a delegate writing
	// into the main agent's memory is the one failure that would quietly undo
	// the flat-context design and still look like it worked.
	if briefMentions(req, "memory") && offersTool(req, "memory") {
		return p.noopMemoryReply(model, req)
	}
	// A chair in the office (COMPANY.md §4) is handed a whole deliverable and
	// hands back a file. It is the one delegate that runs on a *different*
	// desk's manifest than its caller's — the §84 carve-out — and the claim
	// worth checking is that a real file comes out of it, in the caller's
	// session folder, with the work recorded against the chair.
	if briefMentions(req, "office") && offersTool(req, "doc_write") {
		return p.noopOfficeChairReply(model, req)
	}
	const probe = "list"
	result := ""
	ran := false
	answer := ""
	asked := false
	for _, m := range req.Messages {
		if m.Role != RoleTool {
			continue
		}
		switch {
		case strings.EqualFold(m.Name, probe):
			ran, result = true, strings.TrimSpace(m.Content)
		case strings.EqualFold(m.Name, "ask_main"):
			asked, answer = true, strings.TrimSpace(m.Content)
		}
	}

	// Asking is opt-in per brief, the same way the image scenarios are opt-in per
	// keyword: a delegate that asked on every run would turn every other test
	// into a two-step dance for no reason. A brief containing "askmain" gets the
	// ask round first, so the test exercises a real park-and-resume — the list
	// round below then happens only after an answer comes back, on the same agent
	// with the same context.
	if !asked && offersTool(req, "ask_main") && briefMentions(req, "askmain") {
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID:       "noop_delegate_ask",
			Type:     "function",
			Function: FunctionCall{Name: "ask_main", Arguments: `{"question":"[tools-test] list the sandbox root, or stop here?"}`},
		}}}
	}

	if !ran && offersTool(req, probe) {
		return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
			ID:       "noop_delegate_1",
			Type:     "function",
			Function: FunctionCall{Name: probe, Arguments: `{"path":"."}`},
		}}}
	}

	if !ran {
		// Nothing read-only was offered at all — say so instead of inventing work.
		return Response{Provider: p.Name(), Model: model, Text: p.pick(
			"[tools-test] ไม่ได้รับเครื่องมืออ่านข้อมูลเลย จึงไม่มีอะไรให้รายงาน",
			"[tools-test] I was handed no read-only tool, so there is nothing to report")}
	}
	reply := p.pick("[tools-test] เรียก list แล้ว ผลที่ได้: ", "[tools-test] ran list, result: ") + clipNoop(result, 400)
	if asked {
		// Echoing the answer proves the delegate resumed the same run rather than
		// being restarted: a fresh agent would have no memory of having asked.
		reply += p.pick("\nคำตอบจากเมนเอเจน: ", "\nmain agent answered: ") + clipNoop(answer, 200)
	}
	return Response{Provider: p.Name(), Model: model, Text: reply}
}

// noopOfficeChairReply is a chair doing its job: one writer call, then the
// receipt. Two rounds, because that is the whole shape of the work a desk may
// hand to another one — a single brief in, a file out — and a script that did
// more here would be testing the file tools rather than the door.
func (p *NoopProvider) noopOfficeChairReply(model string, req Request) Response {
	for _, m := range req.Messages {
		if m.Role == RoleTool && strings.EqualFold(m.Name, "doc_write") {
			return Response{Provider: p.Name(), Model: model, Text: p.pick(
				"[tools-test] เขียนเอกสารเสร็จแล้ว: ", "[tools-test] the document is written: ") +
				clipNoop(strings.TrimSpace(m.Content), 300)}
		}
	}
	return Response{Provider: p.Name(), Model: model, ToolCalls: []ToolCall{{
		ID:   "noop_chair_doc",
		Type: "function",
		Function: FunctionCall{Name: "doc_write", Arguments: p.pick(
			`{"path":"office-demo.docx","blocks":[{"type":"heading","text":"สรุปงานจากออฟฟิศ"},{"type":"paragraph","text":"เขียนโดยเก้าอี้ในออฟฟิศระหว่างชุดทดสอบ"}]}`,
			`{"path":"office-demo.docx","blocks":[{"type":"heading","text":"Office job summary"},{"type":"paragraph","text":"Written by a chair in the office during the test set."}]}`)},
	}}}
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
