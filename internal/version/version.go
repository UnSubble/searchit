package version

const Name = "searchit"

// Defined as a variable so it can be overridden via -ldflags at build time.
var Version = "0.1.0-alpha"

func String() string {
	return Name + " v" + Version
}
