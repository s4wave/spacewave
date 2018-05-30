# Auth

> Cross-language authentication strategies for the Aperture network.

## Introduction

Auth declares a common interface for authentication strategies, used by other system components.

This package also includes cross-platform implementations of a few authentication strategies. Each implementation lives in a sub-package.

## Go Implementation

Each sub-package contains an implementation that will be registered globally when the package is initialized. This means that you can selectively compile in encryption types with an import statement like so:

```go
import (
	"github.com/aperturerobotics/auth"

    // Register specific algorithms.
	_ "github.com/aperturerobotics/auth/scrypt-user-pass"
    
    // alternatively, register all algorithms
	_ "github.com/aperturerobotics/auth/all"
)
```

