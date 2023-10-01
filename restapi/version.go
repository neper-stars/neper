// This file is safe to edit. Once it exists it will not be overwritten

package restapi

import (
	"encoding/json"

	"orus.io/orus-io/go-orusapi"
)

var (
	VCSCommit     string
	VersionTag    = "dev"
	injectVersion = orusapi.InjectVersion(VersionTag, VCSCommit)
)

func GetSwaggerJSON() json.RawMessage {
	return injectVersion(SwaggerJSON)
}

func GetFlatSwaggerJSON() json.RawMessage {
	return injectVersion(FlatSwaggerJSON)
}

func GetVersion() string {
	return orusapi.GetVersion(GetSwaggerJSON())
}
