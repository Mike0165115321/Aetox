package main

// What device the page thinks it is on.
//
// The workbench has had a device menu since the browser pane did, and until now
// it did exactly one thing: shrink the native window to the device's width and
// zoom the page by the same factor, so the CSS viewport measures 390 or 412 for
// real and the page's own media queries fire the way they would on the phone.
//
// That is honest as far as it goes, and it is where it stops being enough. A
// site that asks the *browser* what it is — the user agent, whether there is a
// touch screen, how many device pixels there are to a CSS one — was still being
// told "a desktop on Windows", because nothing in a window's width says
// otherwise. So a page with a real mobile build kept serving the desktop one
// into a 390px window, which looks like a broken responsive layout and is not:
// it is the wrong page, rendered correctly.
//
// Three CDP overrides close that, and they are the same three Chrome's own
// device toolbar sets. Aetox already speaks CDP to its engine (tabView's
// callEngine, which the deck export has ridden since it existed), so this is not
// a new capability. It is three calls we were not making.
//
// The geometry stays where it is, in BrowserPane. The window really is phone
// shaped, which is worth keeping for the person watching: emulation that existed
// only inside the engine would look like a desktop pane claiming to be a phone.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// deviceProfile is one row of the device menu. Sizes are CSS pixels, the same
// numbers the menu has always shown.
//
// The agent had an action for this for a few hours on 31 ส.ค. and it is gone
// again, at the owner's word, after five separate defects in one afternoon:
// incomplete client hints, a viewport set twice, a successful result reported as
// an error, a touch point count the protocol refuses, and an emulation nothing
// was sized to. Every one of them lived in the gap between the stub these tests
// use and the engine the app actually talks to, and every one was found by the
// owner rather than by anything here. The menu keeps all of it because a person
// picking a device can see immediately whether it worked; an agent could not,
// and neither could I.
type deviceProfile struct {
	Name string `json:"name"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	// DPR is devicePixelRatio. A phone that reports 1 gets served the @1x
	// images no phone has been served since 2012, which is its own wrong page.
	DPR float64 `json:"dpr"`
	// Mobile drives the layout mode CDP calls `mobile`: viewport meta honoured,
	// text autosizing on, scrollbars overlaid rather than reserved.
	Mobile bool `json:"mobile"`
	// ua is the user agent string, empty on the profiles that keep the engine's
	// own or derive from it. Unexported: the menu has no use for it, and a
	// binding that shipped it would be inviting the frontend to have opinions
	// about user agents.
	ua string
	// platform is what navigator.platform and the Sec-CH-UA-Platform client
	// hint report. Sites increasingly read the hint rather than the string, and
	// one without the other is a browser that contradicts itself.
	platform string
	// platformVersion is the OS version the client hints report. Required by
	// the protocol rather than optional: see deviceUA on all-or-nothing.
	platformVersion string
}

// iOS user agents are literals because they have to be: nothing about Safari on
// a phone can be derived from the engine running on this desktop. They age with
// iOS rather than with us, which is the slower of the two clocks.
const (
	uaIPhone = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1"
	uaIPad   = "Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1"
)

// browserDevices is the list, and it is the ONLY list.
//
// It lived in Workbench.svelte until the agent needed to name a device too, at
// which point a copy in TypeScript would have been a second place answering
// "what devices are there" — the debt this project keeps refusing. The menu
// reads it back through BrowserDevices.
var browserDevices = []deviceProfile{
	{Name: "Galaxy S8+", W: 360, H: 740, DPR: 4, Mobile: true, platform: "Android", platformVersion: "15"},
	{Name: "iPhone SE", W: 375, H: 667, DPR: 2, Mobile: true, ua: uaIPhone, platform: "iOS", platformVersion: "18.5"},
	{Name: "iPhone 12 Pro", W: 390, H: 844, DPR: 3, Mobile: true, ua: uaIPhone, platform: "iOS", platformVersion: "18.5"},
	{Name: "Pixel 7", W: 412, H: 915, DPR: 2.625, Mobile: true, platform: "Android", platformVersion: "15"},
	{Name: "iPhone 14 Pro Max", W: 430, H: 932, DPR: 3, Mobile: true, ua: uaIPhone, platform: "iOS", platformVersion: "18.5"},
	{Name: "iPad Mini", W: 768, H: 1024, DPR: 2, Mobile: true, ua: uaIPad, platform: "iOS", platformVersion: "18.5"},
	{Name: "iPad Pro", W: 1024, H: 1366, DPR: 2, Mobile: true, ua: uaIPad, platform: "iOS", platformVersion: "18.5"},
	{Name: "Desktop", W: 1280, H: 800, DPR: 1, Mobile: false, platform: "Windows"},
}

// BrowserDevices is the Wails binding the workbench menu builds itself from.
func (a *App) BrowserDevices() []deviceProfile { return browserDevices }

// deviceNamed finds a profile by the name the menu shows. Case-insensitive:
// the name arrives as text across a binding, and a person would forgive that.
func deviceNamed(name string) (deviceProfile, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, d := range browserDevices {
		if strings.ToLower(d.Name) == want {
			return d, true
		}
	}
	return deviceProfile{}, false
}

// androidUA rewrites the engine's own user agent into the phone's.
//
// Derived rather than written out, because the alternative is a Chrome version
// number kept in two places: the engine's real one, and a literal here that is
// wrong the first time WebView2 updates. Only the platform token changes, which
// is the only part that is genuinely about the device.
//
// Returns "" when handed something it does not recognise, and the caller reads
// that as "leave the user agent alone" — a wrong UA is worse than the true one.
func androidUA(engine, model string) string {
	open := strings.IndexByte(engine, '(')
	shut := strings.IndexByte(engine, ')')
	if open < 0 || shut < open {
		return ""
	}
	ua := engine[:open+1] + "Linux; Android 15; " + model + engine[shut:]
	// Chrome's phone UA differs from its desktop one by this word, and sites
	// that read the string rather than the hint are reading for this word.
	if i := strings.Index(ua, " Safari/"); i >= 0 {
		ua = ua[:i] + " Mobile" + ua[i:]
	}
	// Edge marks itself at the end. On a phone that suffix names a browser the
	// page is not being shown by.
	if i := strings.Index(ua, " Edg/"); i >= 0 {
		ua = ua[:i]
	}
	return ua
}

// engineUA asks the engine what it calls itself. Needed in both directions:
// Android's user agent is built from it, and going back to Desktop has to
// restore it, since CDP has no "clear the override" for this one.
func engineUA(ctx context.Context, host *browserHost, id string) (string, error) {
	raw, err := callEngineOn(ctx, host, id, "Browser.getVersion", "{}")
	if err != nil {
		return "", err
	}
	var v struct {
		UserAgent string `json:"userAgent"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", err
	}
	return v.UserAgent, nil
}

// deviceMetrics is the size half, and the whole point of it is what it does NOT
// send.
//
// Two things can set a viewport here and only one of them may. The pane already
// shrinks the native window to the device's width and zooms the page to match,
// which is what makes the CSS viewport measure 390 for real AND makes the thing
// on screen phone shaped. Sending width and height to the engine as well set it
// a SECOND time, from inside, and the two did not agree: the engine painted its
// 390-wide viewport into a window that was still the pane's size and left the
// rest white. That white band down the right of YouTube on 31 ส.ค. is what two
// owners of one number looks like.
//
// So: 0 is CDP's own word for "do not override this", and it is what the size
// fields carry. The window owns the size. The engine owns what the size MEANS —
// mobile layout mode and the pixel ratio, neither of which a window width can
// express.
func deviceMetrics(dev deviceProfile) (method, params string) {
	if dev.W <= 0 {
		return "Emulation.clearDeviceMetricsOverride", "{}"
	}
	return "Emulation.setDeviceMetricsOverride", fmt.Sprintf(
		`{"width":0,"height":0,"deviceScaleFactor":%g,"mobile":%t}`, dev.DPR, dev.Mobile)
}

// applyDevice puts one tab into a device's shoes, or takes it back out of them.
//
// A zero deviceProfile means the pane's own size with nothing overridden, which
// is what the menu's เต็มแผง row asks for.
func (a *App) applyDevice(ctx context.Context, id string, dev deviceProfile) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	// A tab with no page yet has no engine to talk to, and callEngineOn would
	// sit on its timeout before saying so — the menu would appear frozen for
	// picking a phone on an empty tab, which is a reasonable thing to do first.
	//
	// Not an error, because nothing has gone wrong: the choice is kept in the
	// tab's viewport and BrowserPane re-sends it with the first navigation.
	if !host.live(id) {
		return nil
	}
	method, metrics := deviceMetrics(dev)
	if _, err := callEngineOn(ctx, host, id, method, metrics); err != nil {
		return err
	}

	// Touch before the user agent: a page that re-reads its capabilities when
	// the UA changes should find the touch screen already there.
	if _, err := callEngineOn(ctx, host, id, "Emulation.setTouchEmulationEnabled", deviceTouch(dev)); err != nil {
		return err
	}

	// The engine's own string is needed for exactly one case: deriving Android's
	// from it. Everything else either carries its own or is clearing the
	// override, and asking for a version nobody is going to read is a round trip
	// through the message pump for nothing.
	var engine string
	if dev.Mobile && dev.ua == "" {
		if engine, err = engineUA(ctx, host, id); err != nil {
			// Nothing to derive from. Leaving the true user agent in place beats
			// inventing one: the true one at least matches the engine that is
			// actually going to render the page.
			return nil
		}
	}
	override := deviceUA(dev, engine)
	if override == "" {
		return nil
	}
	_, err = callEngineOn(ctx, host, id, "Emulation.setUserAgentOverride", override)
	return err
}

// deviceTouch is the parameters for Emulation.setTouchEmulationEnabled.
//
// maxTouchPoints is only sent when the touch screen is being turned ON, and
// that is not tidiness: the field has a minimum of 1, so the obvious spelling
// of switching it off — enabled false, zero points — is refused as Invalid
// parameters, which WebView2 reports as hr=0x80070057 naming nothing. Going
// back to Desktop failed on this every single time.
//
// The lesson underneath it is the one that cost two rounds: the ON path was
// tested against a real engine and the OFF path was not, because turning a
// thing off looks like it cannot be wrong.
func deviceTouch(dev deviceProfile) string {
	if !dev.Mobile {
		return `{"enabled":false}`
	}
	return `{"enabled":true,"maxTouchPoints":5}`
}

// deviceUA is the parameters for Emulation.setUserAgentOverride, or "" for
// "make no call".
//
// Two things here were learned the hard way, both on 31 ส.ค., both against a
// real Chromium rather than by reading the protocol:
//
//  1. userAgentMetadata is all-or-nothing. A partial one — the platform and
//     the mobile flag, which is what every example on the internet shows — is
//     rejected outright as Invalid parameters, and WebView2 reports that as
//     hr=0x80070057 with no clue which parameter it meant. Every field the type
//     declares goes in, or none of them do.
//
//  2. An empty userAgent is how the override is CLEARED. There is no
//     clearUserAgentOverride, and re-sending the engine's real string looks
//     like it works while leaving the client-hint metadata still saying phone.
//     Empty takes both back at once.
//
// Which is why only mobile devices get an override at all: Desktop and เต็มแผง
// are both "this machine, as it really is", and that is what clearing means.
func deviceUA(dev deviceProfile, engine string) string {
	if !dev.Mobile {
		return `{"userAgent":""}`
	}
	ua, brands, full := dev.ua, "[]", ""
	if ua == "" {
		// Android. The user agent and the brand list both carry the engine's
		// own Chrome version rather than a literal, so there is one place the
		// version comes from and it is the engine itself.
		ua = androidUA(engine, dev.Name)
		if ua == "" {
			return ""
		}
		v := chromeVersion(engine)
		if v == "" {
			return ""
		}
		brands = fmt.Sprintf(`[{"brand":"Chromium","version":%s},{"brand":"Google Chrome","version":%s}]`,
			jsonString(majorOf(v)), jsonString(majorOf(v)))
		full = v
	}
	// Empty brands for Apple devices is not an omission: Safari sends no user
	// agent client hints at all, and a phone claiming to be Safari while
	// announcing Chromium brands is a contradiction a site can catch.
	return fmt.Sprintf(
		`{"userAgent":%s,"platform":%s,"userAgentMetadata":{"brands":%s,"fullVersion":%s,"platform":%s,"platformVersion":%s,"architecture":"","model":%s,"mobile":true}}`,
		jsonString(ua), jsonString(dev.platform), brands, jsonString(full),
		jsonString(dev.platform), jsonString(dev.platformVersion), jsonString(dev.Name))
}

// chromeVersion pulls the four-part version out of a Chromium user agent.
func chromeVersion(ua string) string {
	i := strings.Index(ua, "Chrome/")
	if i < 0 {
		return ""
	}
	v := ua[i+len("Chrome/"):]
	if j := strings.IndexByte(v, ' '); j >= 0 {
		v = v[:j]
	}
	return v
}

// majorOf is the first part of a version. Client-hint brands carry the major
// alone, which is the whole point of the brand list: a site that wants the rest
// asks for it.
func majorOf(v string) string {
	if i := strings.IndexByte(v, '.'); i >= 0 {
		return v[:i]
	}
	return v
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// BrowserSetDevice is the menu's binding: the row the user picked, by name, or
// "" for เต็มแผง.
func (a *App) BrowserSetDevice(id, name string) error {
	dev, ok := deviceNamed(name)
	if name != "" && !ok {
		return fmt.Errorf("no device called %q", name)
	}
	return a.applyDevice(context.Background(), id, dev)
}
