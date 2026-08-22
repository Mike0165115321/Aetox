# หน้าทดสอบเบราว์เซอร์

หน้านี้มีไว้ตอบคำถามที่ `go test` ตอบไม่ได้

สคริปต์ที่ `desktop/browser.go` กับ `desktop/browser_log.go` ประกอบขึ้นเป็น JavaScript
และเทสต์ใน Go ตรวจได้แค่ว่า **สตริงหน้าตาถูก** ไม่ได้ตรวจว่ามันทำงานถูกบนหน้าเว็บจริง
สองอย่างนี้ต่างกันจริง ๆ และเคยต่างกันมาแล้ว: ตอน 22 ส.ค. 2569 คอมเมนต์ใน `aetoxTextJS`
เขียนว่าไม่ต้องไล่ shadow root เพราะ `innerText` รายงานข้อความที่เรนเดอร์แล้ว
รันบนหน้านี้แล้วพบว่าไม่จริง `document.body.innerText` ไม่มีข้อความใน shadow root เลย
เทสต์สตริงผ่านหมดทั้งที่ของพัง

## รัน

ต้องมีสองพอร์ต เพราะ iframe ข้าม origin ต้องมาจากพอร์ตอื่นจริง ๆ

```bash
cd desktop/testdata/browser-fixture && python -m http.server 8801 --bind 127.0.0.1
```

```bash
cd desktop/testdata/browser-fixture && python -m http.server 8802 --bind 127.0.0.1
```

แล้วเปิด `http://127.0.0.1:8801/index.html`

เปิดผ่าน `file://` ก็ได้ แต่จะไม่ได้ทดสอบ iframe ข้าม origin และคำขอ network
จะไม่เหมือนของจริง

## หน้านี้มีอะไร และตรวจอะไรได้

| ของในหน้า | ตรวจว่า |
|---|---|
| ปุ่ม 200 ตัว | `read` ชน 150 แล้วรายงานว่าเหลืออีกเท่าไร (`elementsTotal` ต้องเป็น 202) |
| `ส่งข้อความ` ที่ตำแหน่ง 180 | อ่านแบบไม่กรองไม่มีวันเจอ ต้องใช้ `read(filter)` ถึงจะถึง |
| ปุ่มใน shadow root | `read` เห็น และ `aetoxFind` resolve ได้ ส่วน `document.querySelector` หาไม่เจอ |
| iframe พอร์ต 8801 | ข้อความในเฟรมอยู่ในข้อความของหน้า และปุ่มในเฟรมกดได้ |
| iframe พอร์ต 8802 | นับเป็น `frames` แล้วรายงาน ไม่ใช่เงียบ |
| `shadow-text-marker` | อยู่ในข้อความของหน้า (`innerText` ของ body ไม่มี ต้องอ่านผ่านลูกของ shadow root) |
| บล็อกสูง 2400px | `capture` ธรรมดาได้แค่ส่วนบน `capture(full)` ต้องเห็น `bottom-of-page-marker` |
| `console.log/warn/error` ตอนโหลด | `console` เก็บได้ครบ |
| error ที่โยนทิ้งไว้ กับ promise rejection | `console` เก็บได้ ทั้งที่หน้าไม่ได้ log เอง |
| `fetch('missing.json')` | `network` รายงาน 404 |
| `fetch('api?token=SECRET-...')` | `network` ต้องขึ้น `<redacted>` **ห้ามมีคำว่า SECRET โผล่** |
| XHR ไป `child.html` | `network` เก็บฝั่ง XMLHttpRequest ได้ด้วย |
| กด `ส่งข้อความ` | มี `CLICK-LANDED-MAIN` โผล่ท้ายหน้า |
| กด `ปุ่มในเงา` | มี `CLICK-LANDED-SHADOW` โผล่ใน shadow root |
| กด `ปุ่มในเฟรม` | มี `CLICK-LANDED-FRAME` โผล่ในเฟรม |
| กด ref ที่ไม่มีจริง เช่น 999 | ต้อง **error** ว่าไม่มี element นั้น ห้ามตอบว่าคลิกสำเร็จ |

สามแถวที่ชื่อ `CLICK-LANDED-*` มีไว้เพราะรอบทดสอบ 22 ส.ค. 2569 พิสูจน์ไม่ได้ว่าคลิกโดนจริง
เอเจนต้องไปแก้ไฟล์ fixture เองระหว่างเทสต์แล้วคืนค่า ซึ่งไม่ควรต้องทำ
handler พวกนี้ไม่ได้เพิ่มจำนวน element ที่กดได้ ยอดยังเป็น 202 เท่าเดิม

## วิธีเทสสคริปต์โดยไม่ต้องเปิดแอป

สคริปต์ทุกตัวจบด้วยการเรียก `window.chrome.webview.postMessage` วาง stub ดักไว้ก่อนแล้ว
eval สคริปต์ตรง ๆ ในคอนโซลของเบราว์เซอร์ใดก็ได้ที่เป็น Chromium

```js
window.chrome = { webview: { postMessage: function (s) { window.__cap = s; } } };
// วางสคริปต์ที่ Go ประกอบออกมาตรงนี้ แล้ว
JSON.parse(window.__cap);
```

ดึงสคริปต์ตัวจริงออกมาจาก Go ได้ด้วยเทสต์ชั่วคราวที่เรียก `textScript("tok", "")`,
`clickScript("tok", 3)`, `waitScript(...)`, `logScript()`,
`readLogScript("tok", "console")`
แล้วเขียนลงไฟล์
