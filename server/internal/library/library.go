// Package library answers one question: does a title Discover is showing
// already exist on the NAS?
//
// It is deliberately pure — no I/O, no NAS client, no store. Everything it
// needs is handed to it as plain names, and everything it returns is a decision.
// That separation exists because the interesting part of this feature is not
// the listing, it is the MATCHING, which has real edge cases (release years that
// collide, non-Latin scripts, a dozen ways to write a year range) and exactly
// one unacceptable failure mode: marking a title as owned when it is not. A
// false positive makes a user skip something they wanted; a missed match only
// costs them a duplicate they would have downloaded anyway. Keeping the rules
// here lets them be exhaustively table-tested and carry a coverage floor,
// alongside config and syno, rather than hiding behind a NAS call.
//
// The I/O and the caching live in internal/api (library.go), which reads the
// configured parent folders and hands the names in here.
//
// Spec: specs/0008-show-which-discover.
package library
