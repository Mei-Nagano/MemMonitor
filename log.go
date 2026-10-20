package main

// 内存占用记录：按天切分 CSV 落到 %AppData%\MemMonitor\logs\，
// 每条记录带进程名，方便切换监控目标后仍能区分。
// 写入走 bufio，每 20 秒刷一次盘，避免每秒都唤醒磁盘。

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	logKeepDays = 7
	logFlushSec = 20
)

type memLog struct {
	f        *os.File
	w        *bufio.Writer
	day      string
	last     time.Time
	disabled bool
}

var logger memLog

func logDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	d := filepath.Join(dir, "MemMonitor", "logs")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func (l *memLog) openDay(day string) {
	l.closeFile()
	path := filepath.Join(logDir(), day+".csv")

	needHeader := true
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		needHeader = false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		l.disabled = true
		return
	}
	l.f = f
	l.w = bufio.NewWriterSize(f, 8192)
	l.day = day
	if needHeader {
		l.w.WriteString("time,bytes,percent,process\n")
	}
	l.flush()
}

func (l *memLog) closeFile() {
	if l.w != nil {
		l.w.Flush()
		l.w = nil
	}
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

func (l *memLog) flush() {
	if l.w != nil {
		l.w.Flush()
	}
	l.last = time.Now()
}

// write 追加一条采样记录。
func (l *memLog) write(t time.Time, bytes uint64, pct float64, proc string) {
	if l.disabled {
		return
	}
	day := t.Format("2006-01-02")
	if l.w == nil || l.day != day {
		l.openDay(day)
		if l.w == nil {
			return
		}
	}
	name := strings.ReplaceAll(proc, ",", " ")
	fmt.Fprintf(l.w, "%s,%d,%.2f,%s\n", t.Format("2006-01-02 15:04:05"), bytes, pct, name)
	if t.Sub(l.last) >= logFlushSec*time.Second {
		l.flush()
	}
}

// pruneLogs 删除超过保留天数的日志。
func pruneLogs() {
	entries, err := os.ReadDir(logDir())
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -logKeepDays).Format("2006-01-02")
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".csv") || len(name) < 10 {
			continue
		}
		if name[:10] < cutoff {
			_ = os.Remove(filepath.Join(logDir(), name))
		}
	}
}

// ---------------- 导出 ----------------

// exportLogs 把 [from, to] 区间的记录写成一份 CSV。from 为零值时表示不限起点。
func exportLogs(from, to time.Time, path string) (int, error) {
	entries, err := os.ReadDir(logDir())
	if err != nil {
		return 0, err
	}

	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 64*1024)
	defer w.Flush()

	// BOM 让 Excel 正确识别 UTF-8 中文表头
	w.WriteString("\xEF\xBB\xBF")
	w.WriteString("时间,内存占用(字节),内存占用(%),可读大小,进程\n")

	fromKey := ""
	if !from.IsZero() {
		fromKey = from.Format("2006-01-02 15:04:05")
	}
	toKey := to.Format("2006-01-02 15:04:05")

	count := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".csv") || len(name) < 10 {
			continue
		}
		// 文件名就是日期，可以先按天粗筛
		if fromKey != "" && name[:10] < fromKey[:10] {
			continue
		}
		if name[:10] > toKey[:10] {
			continue
		}

		rf, err := os.Open(filepath.Join(logDir(), name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(rf)
		sc.Buffer(make([]byte, 0, 8192), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			if len(line) < 19 || line[0] == 't' { // 跳过表头
				continue
			}
			key := line[:19]
			if fromKey != "" && key < fromKey {
				continue
			}
			if key > toKey {
				continue
			}
			// time,bytes,percent,process
			parts := strings.SplitN(line, ",", 4)
			if len(parts) < 4 {
				continue
			}
			var bytes uint64
			fmt.Sscanf(parts[1], "%d", &bytes)
			w.WriteString(key)
			w.WriteByte(',')
			w.WriteString(parts[1])
			w.WriteByte(',')
			w.WriteString(parts[2])
			w.WriteByte(',')
			w.WriteString(fmtBytes(bytes))
			w.WriteByte(',')
			w.WriteString(parts[3])
			w.WriteByte('\n')
			count++
		}
		rf.Close()
	}
	return count, nil
}

// ---------------- 系统对话框 ----------------

// saveFileDialog 弹出"另存为"对话框，取消时返回空串。
func saveFileDialog(owner uintptr, defName, title string) string {
	buf := make([]uint16, 1024)
	for i, r := range syscall.StringToUTF16(defName) {
		if i >= len(buf)-1 {
			break
		}
		buf[i] = r
	}

	filter := syscall.StringToUTF16("CSV 文件 (*.csv)\x00*.csv\x00所有文件 (*.*)\x00*.*\x00\x00")
	defExt := syscall.StringToUTF16("csv")
	titleP := syscall.StringToUTF16(title)
	initDir := syscall.StringToUTF16(logDir())

	var ofn openFileNameW
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	ofn.Owner = owner
	ofn.Filter = &filter[0]
	ofn.FilterIndex = 1
	ofn.File = &buf[0]
	ofn.MaxFile = uint32(len(buf))
	ofn.InitialDir = &initDir[0]
	ofn.Title = &titleP[0]
	ofn.DefExt = &defExt[0]
	ofn.Flags = 0x00000002 | 0x00000008 | 0x00000800 | 0x00080000 // OVERWRITEPROMPT|NOCHANGEDIR|PATHMUSTEXIST|EXPLORER

	r, _, _ := pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// openFolder 用资源管理器打开日志目录。
func openFolder(path string) {
	op := syscall.StringToUTF16("open")
	p := syscall.StringToUTF16(path)
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(&op[0])), uintptr(unsafe.Pointer(&p[0])), 0, 0, 1)
}
