// SPDX-FileCopyrightText: 2023 Iván Szkiba
// SPDX-FileCopyrightText: 2023 Raintank, Inc. dba Grafana Labs
//
// SPDX-License-Identifier: AGPL-3.0-only
// SPDX-License-Identifier: MIT

package dashboard

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newWebServer(t *testing.T) {
	t.Parallel()

	th := helper(t)

	srv := newWebServer(th.assets.ui, http.NotFoundHandler(), th.proc.logger)

	require.NotNil(t, srv)
	require.NotNil(t, srv.ServeMux)
	require.NotNil(t, srv.eventEmitter)

	addr, err := srv.listenAndServe("127.0.0.1:0")

	require.NoError(t, err)

	base := "http://" + addr.String()

	testLoc := func(loc string) {
		res, eerr := http.Get(base + loc) //nolint:bodyclose,noctx

		require.NoError(t, eerr)
		require.Equal(t, http.StatusOK, res.StatusCode)
	}

	testLoc("/ui/index.html")
	testLoc("/events")
	testLoc("/")

	res, err := http.Get(base + "/no_such_path") //nolint:bodyclose,noctx

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func Test_webServer_used_addr(t *testing.T) {
	t.Parallel()

	th := helper(t)

	srv := newWebServer(th.assets.ui, http.NotFoundHandler(), th.proc.logger)

	addr, err := srv.listenAndServe("127.0.0.1:0")

	require.NoError(t, err)

	_, err = srv.listenAndServe(addr.String())

	require.Error(t, err)
}
