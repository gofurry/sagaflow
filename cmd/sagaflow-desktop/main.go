package main

import (
	"context"
	"fmt"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fynedesktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gofurry/sagaflow/internal/desktop"
	"github.com/gofurry/sagaflow/packaging/icons"
)

var version = "v0.1.0"

const (
	preferenceAutoOpen    = "desktop.auto_open_workbench"
	preferenceCloseQuits  = "desktop.close_quits"
	defaultLauncherWidth  = 760
	defaultLauncherHeight = 760
)

func main() {
	application := fyneapp.NewWithID("io.gofurry.sagaflow")
	icon := fyne.NewStaticResource("sagaflow.png", icons.AppPNG)
	application.SetIcon(icon)

	window := application.NewWindow("SagaFlow 启动器")
	window.Resize(fyne.NewSize(defaultLauncherWidth, defaultLauncherHeight))
	window.SetFixedSize(false)

	controller, err := desktop.NewController(desktop.Options{})
	if err != nil {
		dialog.ShowError(err, window)
	}
	if controller == nil {
		return
	}

	statusDot := canvas.NewCircle(statusColor(desktop.StatusStopped))
	statusDotBox := container.NewGridWrap(fyne.NewSize(11, 11), statusDot)
	statusLabel := widget.NewLabel("内核尚未启动")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	versionLabel := widget.NewLabel("—")
	pidLabel := widget.NewLabel("—")
	addressLabel := widget.NewLabel(desktop.DefaultURL)
	corePathLabel := widget.NewLabel(controller.Snapshot().CorePath)
	corePathLabel.Wrapping = fyne.TextWrapBreak

	var actionMu sync.Mutex
	actionRunning := false
	startButton := newPointerButtonWithIcon("启动内核", theme.MediaPlayIcon(), nil)
	stopButton := newPointerButtonWithIcon("停止内核", theme.MediaStopIcon(), nil)
	restartButton := newPointerButtonWithIcon("重启内核", theme.ViewRefreshIcon(), nil)
	openButton := newPointerButtonWithIcon("打开工作台", theme.ComputerIcon(), nil)

	runAction := func(action func(context.Context) error, openAfter bool) {
		actionMu.Lock()
		if actionRunning {
			actionMu.Unlock()
			return
		}
		actionRunning = true
		actionMu.Unlock()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			actionErr := action(ctx)
			if actionErr == nil && openAfter {
				actionErr = controller.OpenWorkbench()
			}
			if actionErr != nil {
				fyne.Do(func() { dialog.ShowError(actionErr, window) })
			}
			actionMu.Lock()
			actionRunning = false
			actionMu.Unlock()
		}()
	}

	startButton.OnTapped = func() {
		runAction(controller.Start, application.Preferences().BoolWithFallback(preferenceAutoOpen, true))
	}
	stopButton.OnTapped = func() { runAction(controller.Stop, false) }
	restartButton.OnTapped = func() { runAction(controller.Restart, false) }
	openButton.OnTapped = func() {
		if err := controller.OpenWorkbench(); err != nil {
			dialog.ShowError(err, window)
		}
	}

	openDataButton := newPointerButtonWithIcon("数据目录", theme.FolderOpenIcon(), func() {
		if err := controller.OpenDataDirectory(); err != nil {
			dialog.ShowError(err, window)
		}
	})
	openLogsButton := newPointerButtonWithIcon("日志目录", theme.DocumentIcon(), func() {
		if err := controller.OpenLogDirectory(); err != nil {
			dialog.ShowError(err, window)
		}
	})
	clearLogsButton := newPointerButtonWithIcon("清理旧日志", theme.ContentClearIcon(), func() {
		showClearLogsDialog(window, controller)
	})
	doctorButton := newPointerButtonWithIcon("系统诊断", theme.InfoIcon(), func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, doctorErr := controller.Doctor(ctx)
			fyne.Do(func() {
				if doctorErr != nil {
					if result != "" {
						doctorErr = fmt.Errorf("%w\n\n%s", doctorErr, result)
					}
					dialog.ShowError(doctorErr, window)
					return
				}
				showCloseDialog("SagaFlow 系统诊断", result, window)
			})
		}()
	})
	backupButton := newPointerButtonWithIcon("立即备份", theme.DocumentSaveIcon(), func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			result, backupErr := controller.Backup(ctx)
			fyne.Do(func() {
				if backupErr != nil {
					if result != "" {
						backupErr = fmt.Errorf("%w\n\n%s", backupErr, result)
					}
					dialog.ShowError(backupErr, window)
					return
				}
				showBackupDialog(result, window, controller.OpenBackupDirectory)
			})
		}()
	})
	ffmpegButton := newPointerButtonWithIcon("管理 FFmpeg", theme.MediaVideoIcon(), func() {
		showFFmpegManager(window, controller)
	})
	catalogButton := newPointerButtonWithIcon("检查模型目录", theme.DownloadIcon(), func() {
		showCatalogUpdate(window, controller)
	})
	providerButton := newPointerButtonWithIcon("获取厂商密钥", theme.AccountIcon(), func() {
		showProviderHelp(window)
	})

	autoOpen := application.Preferences().BoolWithFallback(preferenceAutoOpen, true)
	autoOpenCheck := newPreferenceCheck("启动内核后自动打开工作台", autoOpen, func(enabled bool) {
		application.Preferences().SetBool(preferenceAutoOpen, enabled)
	})
	closeQuits := application.Preferences().BoolWithFallback(preferenceCloseQuits, true)
	closeQuitsCheck := newPreferenceCheck("点击窗口关闭按钮时直接退出", closeQuits, func(enabled bool) {
		application.Preferences().SetBool(preferenceCloseQuits, enabled)
	})

	logo := canvas.NewImageFromResource(icon)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(72, 72))
	title := canvas.NewText("SagaFlow", color.NRGBA{R: 207, G: 103, B: 49, A: 255})
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 26
	header := container.NewHBox(logo, container.NewVBox(layout.NewSpacer(), title, layout.NewSpacer()))

	statusIndicator := container.NewHBox(widget.NewLabel("状态"), container.NewCenter(statusDotBox))
	statusLine := container.NewHBox(statusLabel, layout.NewSpacer(), statusIndicator)
	details := container.New(layout.NewFormLayout(),
		widget.NewLabel("工作台地址"), addressLabel,
		widget.NewLabel("内核版本"), versionLabel,
		widget.NewLabel("进程 PID"), pidLabel,
		widget.NewLabel("内核位置"), corePathLabel,
	)
	statusSection := container.NewPadded(container.NewVBox(statusLine, details))
	primaryActions := container.NewGridWithColumns(4, startButton, stopButton, restartButton, openButton)
	maintenanceTitle := widget.NewLabelWithStyle("维护", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	maintenance := container.NewPadded(container.NewVBox(
		maintenanceTitle,
		widget.NewLabel("诊断和打开本机目录，不会上传任何数据。"),
		container.NewGridWithColumns(5, doctorButton, backupButton, openDataButton, openLogsButton, clearLogsButton),
	))
	resourcesTitle := widget.NewLabelWithStyle("组件与帮助", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	resources := container.NewPadded(container.NewVBox(
		resourcesTitle,
		widget.NewLabel("安装本机组件、更新内置目录，或打开厂商的官方密钥页面。"),
		container.NewGridWithColumns(3, ffmpegButton, catalogButton, providerButton),
	))
	settingsTitle := widget.NewLabelWithStyle("设置偏好", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	settings := container.NewPadded(container.NewVBox(settingsTitle, autoOpenCheck, closeQuitsCheck))
	footer := container.NewHBox(widget.NewLabel("SagaFlow Desktop "+version), layout.NewSpacer(), widget.NewLabel("AGPL-3.0"))
	mainContent := container.NewVBox(statusSection, primaryActions, maintenance, resources, settings)

	window.SetContent(container.NewBorder(
		header,
		footer, nil, nil,
		mainContent,
	))

	controller.SetOnChange(func(snapshot desktop.Snapshot) {
		fyne.Do(func() {
			statusLabel.SetText(snapshot.Message)
			statusDot.FillColor = statusColor(snapshot.Status)
			statusDot.Refresh()
			addressLabel.SetText(snapshot.URL)
			corePath := snapshot.CorePath
			if corePath == "" {
				corePath = "未找到；请将内核与启动器放在同一目录"
			}
			corePathLabel.SetText(corePath)
			if snapshot.Version == "" {
				versionLabel.SetText("—")
			} else {
				versionLabel.SetText(snapshot.Version)
			}
			if snapshot.PID == 0 {
				pidLabel.SetText("—")
			} else {
				pidLabel.SetText(fmt.Sprintf("%d", snapshot.PID))
			}
			busy := snapshot.Status == desktop.StatusStarting || snapshot.Status == desktop.StatusStopping
			if snapshot.Status == desktop.StatusRunning {
				startButton.Disable()
				openButton.Enable()
			} else if busy {
				startButton.Disable()
				openButton.Disable()
			} else {
				startButton.Enable()
				openButton.Disable()
			}
			if snapshot.Managed && snapshot.Status == desktop.StatusRunning {
				stopButton.Enable()
				restartButton.Enable()
			} else {
				stopButton.Disable()
				restartButton.Disable()
			}
			externalCore := snapshot.Status == desktop.StatusRunning && !snapshot.Managed
			if busy || externalCore || snapshot.CorePath == "" {
				doctorButton.Disable()
				backupButton.Disable()
			} else {
				doctorButton.Enable()
				backupButton.Enable()
			}
		})
	})

	var quitting sync.Once
	quit := func() {
		quitting.Do(func() {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				snapshot := controller.Snapshot()
				if snapshot.Managed {
					_ = controller.Stop(ctx)
				}
				fyne.DoAndWait(func() {
					// App.Quit closes every window. Remove our close interceptor
					// first so that shutdown cannot recursively intercept itself,
					// and keep the window alive until here so Fyne can tear down
					// its system tray icon deterministically.
					window.SetCloseIntercept(nil)
					application.Quit()
				})
			}()
		})
	}
	if desktopApplication, ok := application.(fynedesktop.App); ok {
		desktopApplication.SetSystemTrayIcon(icon)
		desktopApplication.SetSystemTrayWindow(window)
		desktopApplication.SetSystemTrayMenu(fyne.NewMenu("SagaFlow",
			fyne.NewMenuItem("打开工作台", func() {
				if err := controller.OpenWorkbench(); err != nil {
					window.Show()
					dialog.ShowError(err, window)
				}
			}),
			fyne.NewMenuItem("显示启动器", window.Show),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("启动内核", func() {
				runAction(controller.Start, application.Preferences().BoolWithFallback(preferenceAutoOpen, true))
			}),
			fyne.NewMenuItem("停止内核", func() { runAction(controller.Stop, false) }),
			fyne.NewMenuItem("重启内核", func() { runAction(controller.Restart, false) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("退出", quit),
		))
	}
	// SetSystemTrayWindow installs its own "hide on close" interceptor.
	// Apply the user's close preference after tray setup so it is not replaced.
	window.SetCloseIntercept(func() {
		if application.Preferences().BoolWithFallback(preferenceCloseQuits, true) {
			quit()
			return
		}
		window.Hide()
	})

	window.Show()
	go monitor(controller)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		controller.Refresh(ctx)
	}()
	application.Run()
}

func showCloseDialog(title, message string, window fyne.Window) {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(container.NewPadded(label))
	content := container.NewGridWrap(fyne.NewSize(620, 300), scroll)
	closeDialog := dialog.NewCustomWithoutButtons(title, content, window)
	closeButton := newPointerButton("关闭", closeDialog.Hide)
	closeDialog.SetButtons([]fyne.CanvasObject{closeButton})
	closeDialog.Show()
}

func showClearLogsDialog(window fyne.Window, controller *desktop.Controller) {
	title := widget.NewLabelWithStyle("仅清理历史轮转日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	description := widget.NewLabel("将删除超过保留期限或已经轮转的旧日志文件。当前正在写入的日志、数据文件和备份均不会受到影响。")
	description.Wrapping = fyne.TextWrapWord
	hint := widget.NewLabel("这项操作无法撤销，但不会中断正在运行的 SagaFlow 内核。")
	hint.Wrapping = fyne.TextWrapWord
	content := container.NewGridWrap(
		fyne.NewSize(560, 210),
		container.NewPadded(container.NewVBox(title, description, hint)),
	)
	clearDialog := dialog.NewCustomWithoutButtons("清理历史日志", content, window)
	cancelButton := newPointerButton("取消", clearDialog.Hide)
	clearButton := newPointerButtonWithIcon("确认清理", theme.ContentClearIcon(), func() {
		clearDialog.Hide()
		go func() {
			result, clearErr := controller.ClearOldLogs()
			fyne.Do(func() {
				if clearErr != nil {
					dialog.ShowError(clearErr, window)
					return
				}
				showCloseDialog("清理历史日志", fmt.Sprintf("清理完成。\n\n已删除 %d 个历史日志文件，释放 %s。", result.Files, formatBytes(result.Bytes)), window)
			})
		}()
	})
	clearButton.Importance = widget.HighImportance
	clearDialog.SetButtons([]fyne.CanvasObject{cancelButton, clearButton})
	clearDialog.Show()
}

func showBackupDialog(message string, window fyne.Window, openDirectory func() error) {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	content := container.NewGridWrap(fyne.NewSize(560, 180), container.NewPadded(label))
	backupDialog := dialog.NewCustomWithoutButtons("SagaFlow 备份", content, window)
	openButton := newPointerButtonWithIcon("打开备份文件目录", theme.FolderOpenIcon(), func() {
		if err := openDirectory(); err != nil {
			dialog.ShowError(err, window)
			return
		}
		backupDialog.Hide()
	})
	closeButton := newPointerButton("关闭", backupDialog.Hide)
	backupDialog.SetButtons([]fyne.CanvasObject{openButton, closeButton})
	backupDialog.Show()
}

func monitor(controller *desktop.Controller) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		controller.Refresh(ctx)
		cancel()
	}
}

func statusColor(status desktop.Status) color.Color {
	switch status {
	case desktop.StatusRunning:
		return color.NRGBA{R: 81, G: 150, B: 80, A: 255}
	case desktop.StatusStarting, desktop.StatusStopping:
		return color.NRGBA{R: 218, G: 145, B: 60, A: 255}
	case desktop.StatusError:
		return color.NRGBA{R: 204, G: 68, B: 68, A: 255}
	default:
		return color.NRGBA{R: 145, G: 145, B: 145, A: 255}
	}
}
