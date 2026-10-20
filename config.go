package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是持久化配置，退出/修改时写回 %AppData%\MemMonitor\config.json
type Config struct {
	TargetName string `json:"target_name"`
	TargetPid  uint32 `json:"target_pid"`

	ShowIcon  bool `json:"show_icon"`
	ShowName  bool `json:"show_name"`
	ShowPid   bool `json:"show_pid"`
	ShowMem   bool `json:"show_mem"`
	ShowPct   bool `json:"show_pct"`
	ShowSpark bool `json:"show_spark"`

	Tree     bool   `json:"tree"`
	TopMost  bool   `json:"topmost"`
	Alpha    uint8  `json:"alpha"`
	Interval int    `json:"interval_ms"`
	Metric   string `json:"metric"` // private=私有工作集（同任务管理器）, working=工作集

	Width    int32 `json:"width"`     // 卡片逻辑宽度
	Scale    int   `json:"scale"`     // 界面缩放百分比 85/100/115/130
	LockPos  bool  `json:"lock_pos"`  // 锁定位置
	LockSize bool  `json:"lock_size"` // 锁定大小

	AutoRefresh bool `json:"auto_refresh"` // 选择窗口自动刷新进程列表

	LogEnabled   bool `json:"log_enabled"`    // 记录内存占用日志
	AutoCleanPct int  `json:"auto_clean_pct"` // 整机内存占用超过该百分比时自动清理，0=关闭
	AutoCleanCool int `json:"auto_clean_cool"` // 自动清理冷却秒数

	SortBy   int  `json:"sort_by"`   // 0=内存 1=名称
	SortDesc bool `json:"sort_desc"` // 倒序

	PosSet bool  `json:"pos_set"`
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
}

func defaultConfig() Config {
	return Config{
		ShowIcon:  true,
		ShowName:  true,
		ShowPid:   true,
		ShowMem:   true,
		ShowPct:   true,
		ShowSpark: true,
		TopMost:   true,
		Alpha:     236,
		Interval:  1000,
		Metric:    "private",
		Width:     240,
		Scale:     100,
		AutoRefresh: true,
		LogEnabled:  true,
		AutoCleanPct: 0,
		AutoCleanCool: 300,
		SortBy:    0,
		SortDesc:  true,
	}
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	d := filepath.Join(dir, "MemMonitor")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, "config.json")
}

func loadConfig() Config {
	c := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if c.Interval < 200 || c.Interval > 10000 {
		c.Interval = 1000
	}
	if c.Alpha < 25 || c.Alpha > 255 {
		c.Alpha = 236
	}
	if c.Metric != "working" {
		c.Metric = "private"
	}
	if c.Width < 160 {
		c.Width = 240
	}
	if c.Width > cardMaxW {
		c.Width = cardMaxW
	}
	switch c.Scale {
	case 85, 100, 115, 130:
	default:
		c.Scale = 100
	}
	if c.SortBy != 1 {
		c.SortBy = 0
	}
	switch c.AutoCleanPct {
	case 60, 70, 80, 90:
	default:
		c.AutoCleanPct = 0
	}
	if c.AutoCleanCool < 30 {
		c.AutoCleanCool = 300
	}
	return c
}

func saveConfig() {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath(), data, 0o644)
}
