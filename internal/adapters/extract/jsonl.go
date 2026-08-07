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
	off  int64
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
	for {
		start := r.off
		if !r.sc.Scan() {
			if err := r.sc.Err(); err != nil {
				return nil, false, fmt.Errorf("读取 %s 第 %d 行: %w", r.file.Name(), r.line, err)
			}
			return nil, false, nil
		}
		r.line++
		line := r.sc.Bytes()
		r.off = start + int64(len(line)) + 1
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return nil, false, fmt.Errorf("%w: %s 第 %d 行", ErrBadLine, r.file.Name(), r.line)
		}
		return obj, true, nil
	}
}

// Offset 返回当前已成功读取到的字节偏移（最后一行之后）。
func (r *JSONLLineReader) Offset() int64 { return r.off }

// LastCompleteLineOffset 返回文件中最后一个完整换行结束的偏移。
// 若文件以不完整行结尾（无换行符），返回其起始偏移，供下一次续读。
func LastCompleteLineOffset(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := st.Size()
	if size == 0 {
		return 0, nil
	}
	// 读最后一块（最多 64KB）检查是否以换行结尾
	readLen := int64(64 * 1024)
	if size < readLen {
		readLen = size
	}
	buf := make([]byte, readLen)
	if _, err := f.Seek(size-readLen, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, err
	}
	if buf[len(buf)-1] == '\n' {
		return size, nil // 完整换行结尾
	}
	// 不完整尾行：从最后一块中找最后一个换行符
	lastNL := -1
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] == '\n' {
			lastNL = i
			break
		}
	}
	if lastNL < 0 {
		return 0, nil // 无换行，整个文件是单条不完整行
	}
	return size - readLen + int64(lastNL) + 1, nil
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
		// 返回已解析的行 + 错误，不丢弃已收集数据
		return objs, errs, r.sc.Err()
	}
	return objs, errs, nil
}
