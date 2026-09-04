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
	// 优先原子替换；若目标文件正被 CAD 预览/转换进程读取导致 rename 失败，
	// 退化为覆盖写内容（占用方通常以共享只读方式打开，不影响写入）。
	if err := os.Rename(temporaryPath, path); err != nil {
		if replaceErr := replaceFileContents(temporaryPath, path); replaceErr != nil {
			return ObjectInfo{}, fmt.Errorf("保存附件失败: %w", replaceErr)
		}
	}
	return ObjectInfo{Key: key, Size: size, MimeType: mimeType, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), ModTime: time.Now()}, nil
}

func replaceFileContents(temporaryPath, path string) error {
	source, err := os.Open(temporaryPath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
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

// List returns a read-only inventory of physical objects under the local
// storage root. Temporary files are excluded because they are not addressable
// storage keys and may be left briefly during an atomic write.
func (storage *LocalStorage) List(ctx context.Context) ([]ObjectInfo, error) {
	if storage == nil {
		return nil, errors.New("附件存储未配置")
	}
	root, err := filepath.Abs(storage.root)
	if err != nil {
		return nil, fmt.Errorf("解析附件根目录失败: %w", err)
	}
	objects := make([]ObjectInfo, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		objects = append(objects, ObjectInfo{Key: filepath.ToSlash(relative), Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描附件物理对象失败: %w", err)
	}
	return objects, nil
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
