module github.com/Mike0165115321/Aetox/cli

go 1.25.0

require (
	github.com/Mike0165115321/Aetox/engine v0.0.0
	github.com/Mike0165115321/Aetox/providers v0.0.0
	golang.org/x/term v0.43.0
)

replace (
	github.com/Mike0165115321/Aetox/engine => ../engine
	github.com/Mike0165115321/Aetox/providers => ../providers
)
