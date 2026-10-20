# MemMonitor

极简的 Windows 内存占用监控。一个常驻桌面的小卡片，盯着你关心的那个进程。

纯 Go + `syscall` 动态调用 Win32，**无 cgo、无第三方依赖**，编译出来是单个约 3 MB 的 exe。

## 特性

**监控**

- 悬浮卡片，可置顶、可拖动、可锁定位置和大小
- 按进程名绑定，进程退出后重新启动会自动重新接上（PID 变了也不影响）
- 可选显示：进程图标 / 进程名 / PID / 内存占用 / 内存占比 / 趋势图
- 进程树模式：汇总所有子孙进程的内存
- 内存口径与任务管理器「内存」列一致（私有工作集），也可切换成含共享页的工作集
- 透明背景，10%~100% 可调；透明度只作用于背景，文字始终清晰

**内存清理**

| 档位 | 做法 | 权限 |
| --- | --- | --- |
| 清理当前进程 | `EmptyWorkingSet`，开了进程树会递归子进程 | 普通 |
| 清理所有进程 | 遍历全部可访问进程逐个 `EmptyWorkingSet` | 普通 |
| 清理系统缓存 | 刷系统文件缓存 + 清空备用列表（`NtSetSystemInformation`） | 管理员 |

还能设置「整机内存超过 60/70/80/90% 时自动清理」，带 5 分钟冷却。

**内存记录**

- 每秒采样一次，按天写入 `%AppData%\MemMonitor\logs\YYYY-MM-DD.csv`，保留 7 天
- 右键菜单可导出最近 1 小时 / 6 小时 / 24 小时 / 7 天 / 全部记录为 CSV（带 BOM，Excel 直接打开不乱码）

## 使用

1. 运行 `memmon.exe`
2. **双击卡片**选择要监控的进程
3. **右键卡片**打开菜单：清理内存、内存记录、显示项、内存口径、刷新间隔、不透明度、界面大小、锁定位置/大小、退出

设置自动保存在 `%AppData%\MemMonitor\config.json`。

## 编译

需要 Go 1.21+（无第三方依赖，不需要 `go mod download`）：

```bash
go build -trimpath -ldflags "-s -w -H windowsgui" -o memmon.exe .
```

Windows 上也可以直接双击 `build.bat`。

## 实现要点

- **分层窗口**：`CreateDIBSection` + `UpdateLayeredWindow`（`ULW_ALPHA`），像素必须是预乘 alpha
- **圆角抗锯齿**：不用 `SetWindowRgn`（只有 1 位裁剪，会锯齿），改成逐像素覆盖率遮罩
- **低透明度下文字不糊**：内容层与遮罩层分开渲染再合成——遮罩层用白色画一遍得到覆盖率，内容层用正常颜色画，最后按 `a = at + bgA*(1-at)` 合成预乘输出，文字始终保持不透明
- **字体用灰度抗锯齿**：分层窗口上开 ClearType 会让次像素覆盖率对不上，出现彩色边缘
- **内存口径**：`PROCESS_MEMORY_COUNTERS_EX2.PrivateWorkingSetSize`，失败时逐级降级到 EX / 基础结构
- **图标**：`SHGetImageList(SHIL_EXTRALARGE)` + `IImageList::GetIcon`，取 48×48 再缩放到 26 逻辑像素

## 目录

```
main.go      窗口过程、消息循环、命令行自测
ui.go        卡片渲染、布局、右键菜单
picker.go    自绘的进程选择窗
render.go    离屏表面、抗锯齿圆角、分层窗口提交
proc.go      进程枚举、内存采样、图标提取
clean.go     内存清理（工作集 / 系统缓存 / 备用列表）
log.go       占用记录落盘与导出
config.go    配置持久化
win32.go     全部 Win32 绑定
tools/       开发时用的截图、放大、菜单驱动等小工具
```

## 已知限制

- 仅支持 Windows（大量 Win32 调用）
- 「清理系统缓存」需要以管理员身份运行，否则菜单里会提示权限不足
- `EmptyWorkingSet` 是把页换出到页面文件，任务管理器里的数字会立刻下降，但提交量不变——这是这类"内存优化"的固有性质
