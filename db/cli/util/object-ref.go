package cliutil

import (
	"errors"
	"os"

	ucli "github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/bucket"
)

// RunParseObjectRef parses the object ref provided.
func (a *UtilArgs) RunParseObjectRef(cctx *ucli.Context) error {
	// Require and parse the object reference argument.
	if a.ObjectRef == "" {
		return errors.New("object ref must be specified")
	}
	oref, err := bucket.ParseObjectRef(a.ObjectRef)
	if err != nil {
		return err
	}

	// Print the bucket identity.
	os.Stdout.WriteString("Bucket ID: ")
	if obid := oref.GetBucketId(); obid != "" {
		os.Stdout.WriteString(obid)
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")

	// Print the inline transform configuration.
	os.Stdout.WriteString("Transform Config: ")
	if tc := oref.GetTransformConf(); !tc.GetEmpty() {
		os.Stdout.WriteString(tc.String())
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")

	// Print the transform configuration reference.
	os.Stdout.WriteString("Transform Config Ref: ")
	if tcr := oref.GetTransformConfRef(); !tcr.GetEmpty() {
		os.Stdout.WriteString(tcr.MarshalString())
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")

	// Print the root reference.
	os.Stdout.WriteString("Root Ref: ")
	if tcr := oref.GetRootRef(); !tcr.GetEmpty() {
		os.Stdout.WriteString(tcr.MarshalString())
	} else {
		os.Stdout.WriteString("<none>")
	}
	os.Stdout.WriteString("\n")
	return nil
}
