---
name: aetox-mcp
description: หาเซิร์ฟเวอร์ MCP มาต่อให้ผู้ใช้, ถามก่อนว่าต้องการต่อกับอะไร, คัดตัวที่ Aetox ทำเองได้อยู่แล้วออก, ตัดตัวที่ต้อง OAuth ทิ้ง (ยังไม่รองรับ), แล้วยื่นค่าที่ต้องกรอกให้ผู้ใช้เติมเอง เพราะไฟล์ตั้งค่า MCP ปิดจากเครื่องมือไฟล์ทุกตัว
---

The user has asked you to find them an MCP server. Unlike a skill, this one you
cannot install: you research it, judge it, and hand the user exactly what to
type. What follows is what makes that judgment right in this app.

## The one fact that decides everything: an MCP server is not free

A skill costs nothing until you open it. **An MCP server's tools are sent in
every request, forever, from the moment it connects.** They join the tool block
beside `read` and `shell` and are paid on every message of every session,
whether or not the topic ever comes up.

That is the opposite economics from a skill, and it is why the bar below is high
and why "it looked interesting" is not a reason to add one. A server with
fifteen tools can quietly become the largest thing in the block.

Say this to the user in plain words when they are choosing. It is the trade they
are actually making, and nobody else will tell them.

## The bar, three tests, all three

This is the same bar the curated shelf on the MCP settings page is held to. Use
it on anything you find.

1. **It reaches something Aetox has no tool for.** No server for files, for
   fetching a page, for searching the web, for driving a browser, for running
   commands, all of those are already here, and a second road to the same place
   is slower *and* costs tool-block tokens forever. What passes: a service with
   an account behind it, a specialist engine, somebody's data that lives
   somewhere else.
2. **It connects with what this app can send.** Aetox's MCP client carries
   **static headers only**. A server that wants an OAuth flow cannot be
   connected here at all, recommending one produces a settings form asking for
   a token the user has no way to obtain. Rule it out early and say why.
3. **The endpoint answered, rather than being remembered.** A URL you recalled
   from training is a guess. Verify it with `web_fetch` or by reading the
   project's own current docs before you put it in front of the user.

## Look at what is already on the shelf first

**ตั้งค่า → MCP** ships a curated list with one-click add. Check it before
searching: if what the user needs is there, the answer is two clicks and no
research. Tell them that instead of finding a second way to the same server.

## What you cannot do, and what to do instead

`mcp-servers.json` is **refused to every file tool**, `read`, `write` and
`shell` all come back with a refusal, deliberately: its `headers` and
`environment` maps are where an API key ends up, and that file is closed the
same way the credentials file is.

There is also no tool for adding a server. So the shape of this job is:

1. Find the server and verify its endpoint.
2. Work out the exact fields, see below.
3. Hand them over and say where they go: **ตั้งค่า → MCP → เพิ่มเซิร์ฟเวอร์**.
4. Offer to check afterwards: once it connects, its tools appear to you, and you
   can say out loud what arrived.

### If it needs a key, ask for it

**Ask before you take it, every time.** A key is the user's to hand over, and a
handover they did not agree to is the one thing in this job they cannot undo.

Then say why you are asking and where it ends up. The `aetox` skill's "What
stays here, and what leaves" is the true version of that, including the part
about the model, read it rather than reassuring from memory, and work out how
to say it for the person in front of you.

## The two shapes a server comes in

**Hosted (HTTP)**, a URL, optionally with headers:

```
url:     https://example.com/mcp
headers: Authorization: Bearer ${env:EXAMPLE_API_KEY}
```

**Local (stdio)**, a command Aetox runs on this machine:

```
command:     npx -y @scope/some-mcp-server
cwd:         (optional, where to run it)
environment: SOME_API_KEY=${env:SOME_API_KEY}
```

A local server means installing somebody's package and running it. Say that out
loud, the user is agreeing to run code on their machine, which is a different
decision from adding a URL.

### `${env:NAME}` is the way to carry a secret

Both `headers` and `environment` expand `${env:NAME}` from the environment at
connect time. **Recommend that form rather than the literal key**: the config
then holds a reference instead of a credential, and the file stays safe to back
up or share. Tell the user which variable name to set, and that
`<DataRoot>/.env` is a place they can set it.

If the reference resolves to nothing, the server fails to authenticate and says
so, a clear error, which is better than a key sitting in a file.

## Report it as a decision, not a list

For each server you recommend: what it reaches, why the tools here cannot,
whether it needs a key, and roughly how many tools it will add to every request.
Then let the user choose. One well-argued recommendation beats five names.

If nothing passes the bar, say so and say what you can do without it. That is a
real answer; a marginal server added to have added something is not.
