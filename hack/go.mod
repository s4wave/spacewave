module github.com/aperturerobotics/genproto/tools

go 1.16

replace (
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.8.2-0.20220322010420-77ab346a2cf8 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.27.2-0.20220603060022-78b627edc1c2 // aperture
)

require (
	github.com/golangci/golangci-lint v1.44.2
	github.com/planetscale/vtprotobuf v0.3.0
	github.com/psampaz/go-mod-outdated v0.8.0
	github.com/square/goprotowrap v0.0.0-20210611190042-204ec2527e6f
	google.golang.org/protobuf v1.27.1
	storj.io/drpc v0.0.29
)
