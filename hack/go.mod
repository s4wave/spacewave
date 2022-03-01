module github.com/aperturerobotics/genproto/tools

go 1.16

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/protobuf => github.com/paralin/protobuf-go v1.27.2-0.20220104043425-56bf363d0a34 // aperture
)

require (
	github.com/golang/protobuf v1.5.2
	github.com/golangci/golangci-lint v1.44.2
	github.com/square/goprotowrap v0.0.0-20210611190042-204ec2527e6f
	storj.io/drpc v0.0.29
)
