package httpreaderat_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xakep666/httpreaderat/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPReaderAt(t *testing.T) {
	var (
		content = make([]byte, 10*1024) // 10KiB content sample
		modTime = time.Now().UTC()
	)

	_, err := io.ReadFull(rand.Reader, content)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/ranged-content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "some-random-bytes")
		http.ServeContent(w, r, "content", modTime, bytes.NewReader(content))
	})
	mux.HandleFunc("/not-ranged-content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)

		if r.Method != http.MethodHead {
			_, _ = io.Copy(w, bytes.NewReader(content))
		}
	})

	server := httptest.NewServer(mux)

	t.Run("basic scenario", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/ranged-content", nil)
		require.NoError(t, err)

		rd, err := httpreaderat.New(server.Client(), req, nil)
		require.NoError(t, err)

		assert.Equal(t, int64(len(content)), rd.Size())
		assert.Equal(t, modTime.Format(http.TimeFormat), rd.LastModified())
		assert.Equal(t, "some-random-bytes", rd.ContentType())
		// read 1 chunk in ~middle of content

		chunk := make([]byte, 2*1024)
		off := int64(4 * 1024)
		n, err := io.ReadFull(io.NewSectionReader(rd, off, int64(len(chunk))), chunk)
		if assert.NoError(t, err) {
			assert.Equal(t, len(chunk), n)
			assert.Equal(t, content[off:off+int64(len(chunk))], chunk)
		}

		// read full and compare
		readContent, err := ioutil.ReadAll(io.NewSectionReader(rd, 0, rd.Size()))
		if assert.NoError(t, err) {
			assert.Equal(t, content, readContent)
		}
	})

	t.Run("no range support on server without store", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/not-ranged-content", nil)
		require.NoError(t, err)

		_, err = httpreaderat.New(server.Client(), req, nil)
		assert.True(t, errors.Is(err, httpreaderat.ErrNoRange), "unexpected error", err)
	})

	t.Run("no range support on server with store", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/not-ranged-content", nil)
		require.NoError(t, err)

		rd, err := httpreaderat.New(server.Client(), req, httpreaderat.NewStoreMemory())
		require.NoError(t, err)

		assert.Equal(t, int64(len(content)), rd.Size())
		assert.Equal(t, modTime.Format(http.TimeFormat), rd.LastModified())
		// read 1 chunk in ~middle of content

		chunk := make([]byte, 2*1024)
		off := int64(4 * 1024)
		n, err := io.ReadFull(io.NewSectionReader(rd, off, int64(len(chunk))), chunk)
		if assert.NoError(t, err) {
			assert.Equal(t, len(chunk), n)
			assert.Equal(t, content[off:off+int64(len(chunk))], chunk)
		}

		// read full and compare
		readContent, err := ioutil.ReadAll(io.NewSectionReader(rd, 0, rd.Size()))
		if assert.NoError(t, err) {
			assert.Equal(t, content, readContent)
		}
	})

	t.Run("unexpected code", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/non-existing", nil)
		require.NoError(t, err)

		_, err = httpreaderat.New(server.Client(), req, nil)
		assert.Equal(t, &httpreaderat.ErrUnexpectedResponseCode{Code: http.StatusNotFound}, err)
	})

	t.Run("requested more data than server could give", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/ranged-content", nil)
		require.NoError(t, err)

		rd, err := httpreaderat.New(server.Client(), req, nil)
		require.NoError(t, err)

		chunk := make([]byte, 2*1024)
		off := int64(9 * 1024)
		n, err := rd.ReadAt(chunk, off)
		if assert.True(t, errors.Is(err, io.EOF), "EOF should be returned but got", err) {
			assert.Equal(t, int64(len(content))-off, int64(n))
			assert.Equal(t, chunk[:n], content[off:])
		}
	})
}

func TestHTTPReaderAt_correctly_passes_headers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Header.Get("Authorization"), "Bearer test")
		assert.Equal(t, r.Header.Get("X-Header-1"), "value1")

		_, _ = fmt.Fprint(w, "some data")
	}))

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-Header-1", "value1")

	_, err = httpreaderat.New(server.Client(), req, httpreaderat.NewStoreMemory())
	assert.NoError(t, err)
}
