package httpreaderat_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"io/ioutil"
	"testing"

	"github.com/xakep666/httpreaderat"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicStores(t *testing.T) {
	content := make([]byte, 10*1024) // 10KiB content sample

	_, err := io.ReadFull(rand.Reader, content)
	require.NoError(t, err)

	f := func(store httpreaderat.Store) {
		t.Helper()

		read, err := store.ReadFrom(bytes.NewReader(content))
		require.NoError(t, err)
		require.Equal(t, int64(len(content)), read)

		assert.Equal(t, int64(len(content)), store.Size())

		// read 1 chunk in ~middle of content
		chunk := make([]byte, 2*1024)
		off := int64(4 * 1024)
		n, err := io.ReadFull(io.NewSectionReader(store, off, int64(len(chunk))), chunk)
		if assert.NoError(t, err) {
			assert.Equal(t, len(chunk), n)
			assert.Equal(t, content[off:off+int64(len(chunk))], chunk)
		}

		// read full and compare
		readContent, err := ioutil.ReadAll(io.NewSectionReader(store, 0, int64(len(content))))
		if assert.NoError(t, err) {
			assert.Equal(t, content, readContent)
		}

		assert.NoError(t, store.Close())
	}

	f(httpreaderat.NewStoreMemory())
	f(httpreaderat.NewStoreFile())
}

type MockedStore struct {
	Content bytes.Buffer
}

func (m *MockedStore) ReadFrom(r io.Reader) (n int64, err error) {
	return m.Content.ReadFrom(r)
}

func (m *MockedStore) ReadAt(p []byte, off int64) (n int, err error) {
	return bytes.NewReader(m.Content.Bytes()).ReadAt(p, off)
}

func (m *MockedStore) Close() error {
	m.Content.Reset()
	return nil
}

func (m *MockedStore) Size() int64 { return int64(m.Content.Len()) }

func TestLimitedStore(t *testing.T) {
	content := make([]byte, 10*1024) // 10KiB content sample

	_, err := io.ReadFull(rand.Reader, content)
	require.NoError(t, err)

	t.Run("content fits limit", func(t *testing.T) {
		var primaryStore MockedStore

		store := httpreaderat.NewLimitedStore(&primaryStore, 20*1024, nil)

		read, err := store.ReadFrom(bytes.NewReader(content))
		require.NoError(t, err)
		require.Equal(t, int64(len(content)), read)

		assert.Equal(t, content, primaryStore.Content.Bytes())
		assert.Equal(t, int64(len(content)), store.Size())

		// read 1 chunk in ~middle of content
		chunk := make([]byte, 2*1024)
		off := int64(4 * 1024)
		n, err := io.ReadFull(io.NewSectionReader(store, off, int64(len(chunk))), chunk)
		if assert.NoError(t, err) {
			assert.Equal(t, len(chunk), n)
			assert.Equal(t, content[off:off+int64(len(chunk))], chunk)
		}

		// read full and compare
		readContent, err := ioutil.ReadAll(io.NewSectionReader(store, 0, int64(len(content))))
		if assert.NoError(t, err) {
			assert.Equal(t, content, readContent)
		}

		assert.NoError(t, store.Close())
	})

	t.Run("content not fits limit and goes to secondary", func(t *testing.T) {
		var (
			primaryStore   MockedStore
			secondaryStore MockedStore
		)

		store := httpreaderat.NewLimitedStore(&primaryStore, 5*1024, &secondaryStore)

		read, err := store.ReadFrom(bytes.NewReader(content))
		require.NoError(t, err)
		require.Equal(t, int64(len(content)), read)

		assert.Empty(t, primaryStore.Content.Bytes())
		assert.Equal(t, content, secondaryStore.Content.Bytes())
		assert.Equal(t, int64(len(content)), store.Size())

		// read 1 chunk in ~middle of content
		chunk := make([]byte, 2*1024)
		off := int64(4 * 1024)
		n, err := io.ReadFull(io.NewSectionReader(store, off, int64(len(chunk))), chunk)
		if assert.NoError(t, err) {
			assert.Equal(t, len(chunk), n)
			assert.Equal(t, content[off:off+int64(len(chunk))], chunk)
		}

		// read full and compare
		readContent, err := ioutil.ReadAll(io.NewSectionReader(store, 0, int64(len(content))))
		if assert.NoError(t, err) {
			assert.Equal(t, content, readContent)
		}

		assert.NoError(t, store.Close())
	})

	t.Run("content not fits limit without secondary", func(t *testing.T) {
		var primaryStore MockedStore

		store := httpreaderat.NewLimitedStore(&primaryStore, 5*1024, nil)

		_, err := store.ReadFrom(bytes.NewReader(content))
		assert.True(t, errors.Is(err, httpreaderat.ErrStoreLimit), "Unexpected error", err)
	})
}
