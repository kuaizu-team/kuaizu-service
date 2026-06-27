package service

import (
	"bytes"
	"io"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryMultipartFile struct{ *bytes.Reader }

func (memoryMultipartFile) Close() error { return nil }

func TestValidateImageContent(t *testing.T) {
	tests := []struct {
		name    string
		ext     string
		data    []byte
		wantErr bool
	}{
		{name: "jpeg", ext: ".jpg", data: append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 508)...)},
		{name: "jpeg extension", ext: ".jpeg", data: append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 508)...)},
		{name: "png", ext: ".png", data: append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...)},
		{name: "heic disguised as jpeg", ext: ".jpg", data: append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), make([]byte, 488)...), wantErr: true},
		{name: "png with jpeg extension", ext: ".jpg", data: append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...), wantErr: true},
		{name: "empty file", ext: ".jpg", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var file multipart.File = memoryMultipartFile{bytes.NewReader(tt.data)}
			err := validateImageContent(file, tt.ext)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			position, seekErr := file.Seek(0, io.SeekCurrent)
			require.NoError(t, seekErr)
			require.Zero(t, position)
		})
	}
}
