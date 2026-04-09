// Package version declares the SDK version for compatibility checking.
package version

// Version is the SDK version. Plugins vendor this module; the host reads
// this constant from the plugin's vendored source to check compatibility
// at load time.
//
// Bump patch for fixes, minor for additive changes, major for breaking changes.
const Version = "0.0.0"
