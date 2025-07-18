package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-enry/go-license-detector/v4/licensedb"
	"github.com/stretchr/testify/assert"
)

func TestCmdMain(t *testing.T) {
	buffer := &bytes.Buffer{}
	detect([]string{"../..", "."}, "json", buffer)
	var r []licensedb.Result
	err := json.Unmarshal(buffer.Bytes(), &r)
	assert.NoError(t, err)
	assert.Len(t, r, 2)
	assert.Equal(t, "../..", r[0].Arg)
	assert.Equal(t, ".", r[1].Arg)
	assert.Len(t, r[0].Matches, 4)
	assert.Len(t, r[1].Matches, 0)
	assert.Equal(t, "", r[0].ErrStr)
	assert.Equal(t, "no license file was found", r[1].ErrStr)
	assert.Equal(t, "Apache-2.0", r[0].Matches[0].License)
	assert.InDelta(t, 0.9877, r[0].Matches[0].Confidence, 0.002)
	assert.Equal(t, "ECL-2.0", r[0].Matches[1].License)
	assert.InDelta(t, 0.9047, r[0].Matches[1].Confidence, 0.002)
	buffer.Reset()
	detect([]string{"../..", "."}, "text", buffer)
	assert.Equal(t, `../..
	99%	Apache-2.0
	90%	ECL-2.0
	81%	SHL-0.51
	81%	SHL-0.5
.
	no license file was found
`, buffer.String())
}
