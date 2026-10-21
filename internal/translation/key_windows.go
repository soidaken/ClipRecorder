package translation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func keyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ClipRecorder", "deepseek-key.dat"), nil
}

func protect(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("密钥内容为空")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, 1, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func LoadKey() (string, error) {
	path, err := keyPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := protect(data, true)
	return string(plain), err
}

func SaveKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 8 || strings.ContainsAny(key, "\r\n\t ") {
		return fmt.Errorf("请输入完整的 DeepSeek API Key")
	}
	data, err := protect([]byte(key), false)
	if err != nil {
		return err
	}
	path, err := keyPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".deepseek-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
