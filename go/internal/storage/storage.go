package storage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ObjectInfo struct {
	Key      string
	Size     int64
	MimeType string
	SHA256   string
	ModTime  time.Time
}

type ObjectStorage interface {
	Put(ctx context.Context, key string, reader io.Reader, mimeType string) (ObjectInfo, error)
	Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	if strings.TrimSpace(root) == "" {
		root = "./storage/attachments"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件存储根目录失败: %w", err)
	}
	return &LocalStorage{root: root}, nil
}

func (storage *LocalStorage) Put(ctx context.Context, key string, reader io.Reader, mimeType string) (ObjectInfo, error) {
	path, err := storage.objectPath(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ObjectInfo{}, fmt.Errorf("创建附件目录失败: %w", err)
	}
	temporaryPath := fmt.Sprintf("%s.%d.tmp", path, time.Now().UnixNano())
	file, err := os.Create(temporaryPath)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("创建附件临时文件失败: %w", err)
	}
	defer os.Remove(temporaryPath)

	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	size, copyErr := copyWithContext(ctx, writer, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return ObjectInfo{}, fmt.Errorf("写入附件失败: %w", copyErr)
	}
	if closeErr != nil {
		return ObjectInfo{}, fmt.Errorf("关闭附件临时文件失败: %w", closeErr)
	}
	backupPath := fmt.Sprintf("%s.%d.bak", path, time.Now().UnixNano())
	hadOriginal := false
	if err := os.Rename(path, backupPath); err != nil {
		if !os.IsNotExist(err) {
			return ObjectInfo{}, fmt.Errorf("备份旧附件失败: %w", err)
		}
	} else {
		hadOriginal = true
	}
	defer func() {
		if hadOriginal {
			_ = os.Remove(backupPath)
		}
	}()
	if err := os.Rename(temporaryPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, path)
		}
		return ObjectInfo{}, fmt.Errorf("保存附件失败: %w", err)
	}
	return ObjectInfo{Key: key, Size: size, MimeType: mimeType, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), ModTime: time.Now()}, nil
}

func (storage *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	path, err := storage.objectPath(key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ObjectInfo{}, os.ErrNotExist
		}
		return nil, ObjectInfo{}, fmt.Errorf("打开附件失败: %w", err)
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, ObjectInfo{}, fmt.Errorf("读取附件信息失败: %w", err)
	}
	return file, ObjectInfo{Key: key, Size: stat.Size(), ModTime: stat.ModTime()}, nil
}

func (storage *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := storage.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除附件失败: %w", err)
	}
	return nil
}

func (storage *LocalStorage) objectPath(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("附件存储键不能为空")
	}
	segments := strings.Split(strings.ReplaceAll(key, "\\", "/"), "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, ":\r\n") {
			return "", errors.New("附件存储键无效")
		}
	}
	path := filepath.Join(append([]string{storage.root}, segments...)...)
	root, err := filepath.Abs(storage.root)
	if err != nil {
		return "", fmt.Errorf("解析附件根目录失败: %w", err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析附件路径失败: %w", err)
	}
	relative, err := filepath.Rel(root, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("附件存储路径越界")
	}
	return absolutePath, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
