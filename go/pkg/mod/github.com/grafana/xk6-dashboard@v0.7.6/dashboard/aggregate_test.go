// SPDX-FileCopyrightText: 2023 Iván Szkiba
// SPDX-FileCopyrightText: 2023 Raintank, Inc. dba Grafana Labs
//
// SPDX-License-Identifier: AGPL-3.0-only
// SPDX-License-Identifier: MIT

package dashboard

import (
	"bufio"
	"compress/gzip"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_aggregate(t *testing.T) {
	t.Parallel()

	th := helper(t).osFs()
	out := filepath.Join(t.TempDir(), "out.ndjson")

	opts := &options{
		Tags:   defaultTags(),
		Period: 2 * time.Second,
	}

	err := aggregate("testdata/result.json", out, opts, th.proc)

	require.NoError(t, err)

	file, err := th.proc.fs.Open(out)

	require.NoError(t, err)

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	require.NoError(t, file.Close())

	// last 2 event (snapshot, cumulative) depends on time relations...
	require.LessOrEqual(t, testdataEvents-2, len(lines))
	require.GreaterOrEqual(t, testdataEvents, len(lines))
}

func Test_aggregate_gzip(t *testing.T) {
	t.Parallel()

	th := helper(t).osFs()
	out := filepath.Join(t.TempDir(), "out.ndjson.gz")

	opts := &options{
		Tags:   defaultTags(),
		Period: 2 * time.Second,
	}

	err := aggregate("testdata/result.json.gz", out, opts, th.proc)

	require.NoError(t, err)

	file, err := th.proc.fs.Open(out)

	require.NoError(t, err)

	reader, err := gzip.NewReader(file)

	require.NoError(t, err)

	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	require.NoError(t, reader.Close())
	require.NoError(t, file.Close())

	// last 2 event (snapshot, cumulative) depends on time relations...
	require.LessOrEqual(t, testdataEvents-2, len(lines))
	require.GreaterOrEqual(t, testdataEvents, len(lines))
}
