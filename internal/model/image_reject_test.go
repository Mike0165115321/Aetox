package model

import (
	"errors"
	"testing"
)

func TestIsImageRejection(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			// Verbatim from the owner's log, 30 ส.ค. 05:18:23 — the whole
			// reason this classifier exists.
			name: "deepseek",
			err: errors.New("deepseek request failed with status 400: messages.66.content[2].image[0]: " +
				"You have uploaded an unsupported image. Please make sure your image is valid and has one of the following formats: webp, png, jpeg, and gif."),
			want: true,
		},
		{name: "anthropic", err: errors.New("anthropic request failed with status 400: Could not process image"), want: true},
		{name: "openai code", err: errors.New("openai request failed with status 400: invalid_image_format"), want: true},
		{name: "over size", err: errors.New("request failed with status 400: image exceeds 8000 pixels"), want: true},

		// Everything below fails for a reason dropping the pictures does not
		// fix, and each one already has its own answer in the tool loop.
		{name: "context length", err: errors.New("anthropic request failed with status 400: prompt is too long: 219398 tokens > 200000 maximum"), want: false},
		{name: "quota", err: errors.New("openai request failed with status 429: insufficient_quota"), want: false},
		{name: "tool block", err: errors.New("request failed with status 400: tools[3].function.parameters: invalid schema"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsImageRejection(c.err); got != c.want {
				t.Fatalf("IsImageRejection = %v, want %v", got, c.want)
			}
		})
	}
}
