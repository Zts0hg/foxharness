// Package settings reads and writes user settings stored in
// ~/.foxharness/settings.json. It provides atomic file writes and a model
// priority resolution helper so that callers never need to handle the raw file
// directly.
package settings
