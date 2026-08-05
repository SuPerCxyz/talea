package extract

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// JSONLLineReader 流式读取 JSONL 文件，逐行容错解析。
type JSONLLineReader struct {
	file *os.File
	sc   *bufio.Scanner
	line int
}

// OpenJSONL 打开 JSONL 文件用于流式读取。
func OpenJSONL(path string) (*JSONLLineReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 %s: %w", path, err)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	return &JSONLLineReader{file: f, sc: sc}, nil
}

// Next 读取下一行并解析为对象。
// 返回 (ok=false) 表示文件结束。单行损坏返回 ErrBadLine 并附带行号。
var ErrBadLine = errors.New("损坏的 JSONL 行")

func (r *JSONLLineReader) Next() (map[string]any, bool, error) {
	if !r.sc.Scan() {
		if err := r.sc.Err(); err != nil {
			return nil, false, fmt.Errorf("读取 %s 第 %d 行: %w", r.file.Name(), r.line, err)
		}
		return nil, false, nil
	}
	r.line++
	line := r.sc.Bytes()
	if len(line) == 0 {
		return r.Next()
	}
	var obj map[string]any
	if err := json.Unmarshal(line, &obj); err != nil {
		return nil, false, fmt.Errorf("%w: %s 第 %d 行", ErrBadLine, r.file.Name(), r.line)
	}
	return obj, true, nil
}

// Close 关闭文件。
func (r *JSONLLineReader) Close() error { return r.file.Close() }

// ReadFileJSON 一次性读取整个 JSONL 文件并容错解析，返回有效行与错误列表。
func ReadFileJSON(path string) ([]map[string]any, []error, error) {
	r, err := OpenJSONL(path)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	var (
		objs []map[string]any
		errs []error
	)
	for {
		obj, ok, err := r.Next()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ok {
			break
		}
		objs = append(objs, obj)
	}
	if r.sc.Err() != nil {
		return nil, nil, r.sc.Err()
	}
	return objs, errs, nil
}

// CopyReader 辅助：将 io.Reader 复制并返回复制后的行错误（预留，未使用时可删）。
func CopyReader(_ io.Reader) {}
