package web_pkg_esbuild

import (
	"os"
	"testing"
)

const expectedShim = `import * as __bldr_react from "react";
import * as __bldr_react_dom from "react-dom";
const require = (pkgName) => {
  switch (pkgName) {
    case "react":
      return __bldr_react;
    case "react-dom":
      return __bldr_react_dom;
    default:
      throw Error('Dynamic require of "' + pkgName + '" is not supported');
  }
};
`

func TestNewImportBannerShim(t *testing.T) {
	shim := NewImportBannerShim([]string{"react", "react-dom"}, false, nil)
	if shim != expectedShim {
		os.Stderr.WriteString(expectedShim)
		os.Stderr.WriteString(shim)
		t.FailNow()
	}
}
