// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import _ "embed"

//go:embed external/highlight.js/highlight.min.js
var highlightScriptAsset []byte

//go:embed external/highlight.js/styles/github.min.css
var highlightStyleAsset []byte
