//go:build !sourcemock

package providers

// Pointing a driver at a fake site is a BUILD-TIME capability, not a runtime
// one. In an ordinary build this function is the only definition that exists and
// it always returns "", so there is no environment variable, config key or admin
// setting that could redirect a driver away from the real site — the outbound
// allowlist stays structural, as the constitution requires.
//
// The counterpart in mockbase_dev.go is compiled only under the `sourcemock`
// build tag, which dev and e2e builds pass and a release build never does.
func mockBase(kind string) string { return "" }
