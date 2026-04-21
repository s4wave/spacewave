//go:build js

package common

import (
	"github.com/sirupsen/logrus"
)

func init() {
	log.Formatter = &logrus.TextFormatter{
		DisableColors: true,
	}
}
