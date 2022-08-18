package cliutil

import (
	"errors"
	"os"

	"github.com/aperturerobotics/hydra/bucket"
	ucli "github.com/urfave/cli/v2"
)

// RunParseObjectRef parses the object ref provided.
func (a *UtilArgs) RunParseObjectRef(cctx *ucli.Context) error {
	if a.ObjectRef == "" {
		return errors.New("object ref must be specified")
	}

	oref, err := bucket.ParseObjectRef(a.ObjectRef)
	if err != nil {
		return err
	}

	os.Stdout.WriteString("Bucket ID: ")
	if obid := oref.GetBucketId(); obid != "" {
		os.Stdout.WriteString(obid)
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")

	os.Stdout.WriteString("Transform Config Ref: ")
	if tcr := oref.GetTransformConfRef(); !tcr.GetEmpty() {
		os.Stdout.WriteString(tcr.MarshalString())
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")

	os.Stdout.WriteString("Root Ref: ")
	if tcr := oref.GetRootRef(); !tcr.GetEmpty() {
		os.Stdout.WriteString(tcr.MarshalString())
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")
	return nil
}
