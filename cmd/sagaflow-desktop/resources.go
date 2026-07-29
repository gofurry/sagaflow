package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gofurry/sagaflow/internal/desktop"
	"github.com/gofurry/sagaflow/internal/providercatalog"
)

func showFFmpegManager(window fyne.Window, controller *desktop.Controller) {
	statusTitle := widget.NewLabelWithStyle("正在检查 FFmpeg…", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusDetail := widget.NewLabel("")
	statusDetail.Wrapping = fyne.TextWrapWord
	progress := widget.NewProgressBar()
	progressText := widget.NewLabel("")

	mode := widget.NewRadioGroup([]string{"直接下载", "代理下载", "手动下载"}, nil)
	mode.Horizontal = true
	mode.SetSelected("直接下载")

	proxyPort := widget.NewEntry()
	proxyPort.SetText("7897")
	proxyUsername := widget.NewEntry()
	proxyUsername.SetPlaceHolder("账户（可选）")
	proxyPassword := widget.NewPasswordEntry()
	proxyPassword.SetPlaceHolder("密码（可选）")
	proxyForm := widget.NewForm(
		widget.NewFormItem("本机代理端口", proxyPort),
		widget.NewFormItem("账户", proxyUsername),
		widget.NewFormItem("密码", proxyPassword),
	)
	proxyHint := widget.NewLabel("仅支持本机 HTTP 代理 127.0.0.1；账户和密码只用于本次下载。")
	proxyHint.Wrapping = fyne.TextWrapWord
	proxyPanel := container.NewVBox(proxyHint, proxyForm)

	directHint := widget.NewLabel("连接固定版本下载源，自动校验 SHA-256，随后解压 FFmpeg 与 FFprobe。")
	directHint.Wrapping = fyne.TextWrapWord
	directPanel := container.NewVBox(directHint)

	manualContent := container.NewVBox()
	manualScroll := container.NewVScroll(container.NewPadded(manualContent))
	manualScroll.SetMinSize(fyne.NewSize(640, 220))

	modeArea := container.NewStack(directPanel, proxyPanel, manualScroll)
	modeAreaWrap := container.NewGridWrap(fyne.NewSize(660, 235), modeArea)
	proxyPanel.Hide()
	manualScroll.Hide()

	content := container.NewVBox(
		statusTitle,
		statusDetail,
		progress,
		progressText,
		widget.NewSeparator(),
		mode,
		modeAreaWrap,
	)
	contentWrap := container.NewGridWrap(fyne.NewSize(680, 500), container.NewPadded(content))
	managerDialog := dialog.NewCustomWithoutButtons("管理 FFmpeg", contentWrap, window)

	startButton := newPointerButtonWithIcon("开始下载", theme.DownloadIcon(), nil)
	cancelButton := newPointerButtonWithIcon("取消下载", theme.CancelIcon(), nil)
	openDirectoryButton := newPointerButtonWithIcon("打开安装目录", theme.FolderOpenIcon(), func() {
		if err := controller.OpenFFmpegDirectory(); err != nil {
			dialog.ShowError(err, window)
		}
	})
	closeButton := newPointerButton("关闭", managerDialog.Hide)
	managerDialog.SetButtons([]fyne.CanvasObject{openDirectoryButton, cancelButton, startButton, closeButton})

	var lastStage string
	updateStatus := func(status desktop.FFmpegStatus) {
		if status.Available {
			statusTitle.SetText("FFmpeg 已就绪")
			version := conciseVersion(status.Version)
			if version == "" {
				version = status.InstallVersion
			}
			statusDetail.SetText(fmt.Sprintf("版本 %s · %s\n%s", version, status.Platform, status.FFmpegPath))
		} else {
			if status.Installing {
				statusTitle.SetText(status.Message)
			} else {
				statusTitle.SetText("FFmpeg 尚未安装")
			}
			statusDetail.SetText(fmt.Sprintf("%s\n目标版本 %s · %s\n安装位置：%s", status.Message, status.InstallVersion, status.Platform, status.InstallDirectory))
		}
		progress.SetValue(status.InstallProgress)
		if status.Installing {
			progressText.SetText(fmt.Sprintf("%s · %s / %s · %s/s%s",
				installStageLabel(status.InstallStage),
				formatBytes(status.DownloadedBytes),
				formatBytes(status.DownloadBytes),
				formatBytes(status.DownloadSpeed),
				formatETA(status.ETASeconds),
			))
			startButton.Disable()
			mode.Disable()
			if status.CanCancel {
				cancelButton.Enable()
			} else {
				cancelButton.Disable()
			}
		} else {
			if status.InstallStage == "failed" || status.InstallStage == "canceled" {
				progressText.SetText(status.Message)
			} else if status.Available {
				progressText.SetText("无需下载；可随时重新检测安装目录。")
			} else {
				progressText.SetText("下载可在后台继续，关闭此窗口不会取消任务。")
			}
			mode.Enable()
			cancelButton.Disable()
			if (!status.InstallSupported || status.Available) && mode.Selected != "手动下载" {
				startButton.Disable()
			} else {
				startButton.Enable()
			}
		}
		if lastStage != status.InstallStage {
			lastStage = status.InstallStage
			managerDialog.Refresh()
		}
	}

	rebuildManual := func(status desktop.FFmpegStatus) {
		items := []fyne.CanvasObject{
			widget.NewLabel("下载与当前系统架构匹配的固定版本压缩包："),
		}
		for _, asset := range status.ManualDownloads {
			downloadURL, err := url.Parse(asset.URL)
			if err != nil {
				continue
			}
			link := widget.NewHyperlink(fmt.Sprintf("%s · %s", asset.Name, formatBytes(asset.Size)), downloadURL)
			hash := widget.NewLabel("SHA-256: " + asset.SHA256)
			hash.TextStyle = fyne.TextStyle{Monospace: true}
			hash.Wrapping = fyne.TextWrapBreak
			items = append(items, link, hash)
		}
		files := strings.Join(status.ManualFiles, " 和 ")
		instructions := widget.NewLabel(fmt.Sprintf("解压后，将 %s 放入：\n%s\n完成后点击“重新检测”。", files, status.InstallDirectory))
		instructions.Wrapping = fyne.TextWrapBreak
		items = append(items, instructions)
		manualContent.Objects = items
		manualContent.Refresh()
	}

	setMode := func(selected string) {
		directPanel.Hide()
		proxyPanel.Hide()
		manualScroll.Hide()
		switch selected {
		case "代理下载":
			proxyPanel.Show()
			startButton.SetText("开始下载")
		case "手动下载":
			manualScroll.Show()
			startButton.SetText("重新检测")
		default:
			directPanel.Show()
			startButton.SetText("开始下载")
		}
		updateStatus(controller.FFmpegStatus())
	}
	mode.OnChanged = setMode

	startButton.OnTapped = func() {
		if mode.Selected == "手动下载" {
			updateStatus(controller.RefreshFFmpeg())
			return
		}
		options := desktop.FFmpegInstallOptions{Mode: "direct"}
		if mode.Selected == "代理下载" {
			port, err := strconv.Atoi(strings.TrimSpace(proxyPort.Text))
			if err != nil || port < 1 || port > 65535 {
				dialog.ShowError(fmt.Errorf("代理端口必须是 1 到 65535 之间的数字"), window)
				return
			}
			options.Mode = "proxy"
			options.ProxyPort = port
			options.ProxyUsername = strings.TrimSpace(proxyUsername.Text)
			options.ProxyPassword = proxyPassword.Text
		}
		status, err := controller.StartFFmpegInstall(options)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		updateStatus(status)
	}
	cancelButton.OnTapped = func() {
		updateStatus(controller.CancelFFmpegInstall())
	}

	initialStatus := controller.RefreshFFmpeg()
	rebuildManual(initialStatus)
	updateStatus(initialStatus)

	done := make(chan struct{})
	var doneOnce sync.Once
	managerDialog.SetOnClosed(func() { doneOnce.Do(func() { close(done) }) })
	managerDialog.Show()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status := controller.FFmpegStatus()
				fyne.Do(func() { updateStatus(status) })
			}
		}
	}()
}

func showCatalogUpdate(window fyne.Window, controller *desktop.Controller) {
	title := widget.NewLabelWithStyle("正在检查模型目录更新…", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	detail := widget.NewLabel("将从 SagaFlow 官方 GitHub Release 获取并校验目录文件。")
	detail.Wrapping = fyne.TextWrapWord
	content := container.NewGridWrap(fyne.NewSize(560, 210), container.NewPadded(container.NewVBox(title, detail)))
	updateDialog := dialog.NewCustomWithoutButtons("模型目录更新", content, window)
	checkButton := newPointerButtonWithIcon("重新检查", theme.ViewRefreshIcon(), nil)
	installButton := newPointerButtonWithIcon("安装更新", theme.DownloadIcon(), nil)
	closeButton := newPointerButton("关闭", updateDialog.Hide)
	installButton.Disable()
	updateDialog.SetButtons([]fyne.CanvasObject{checkButton, installButton, closeButton})

	var checked desktop.CatalogUpdateCheck
	check := func() {
		checkButton.Disable()
		installButton.Disable()
		title.SetText("正在检查模型目录更新…")
		detail.SetText("正在连接 SagaFlow 官方 GitHub Release。")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := controller.CheckModelCatalog(ctx)
			fyne.Do(func() {
				checkButton.Enable()
				if err != nil {
					title.SetText("无法检查模型目录")
					detail.SetText(err.Error())
					return
				}
				checked = result
				if result.UpdateAvailable {
					title.SetText(fmt.Sprintf("发现模型目录 %s", result.RemoteVersion))
					detail.SetText(fmt.Sprintf(
						"当前 %s（%d 个模型）→ 新版 %s（%d 个模型）\n新增 %d · 修改 %d · 退役 %d · 移除 %d\n安装后将在下次启动内核时同步，上一版目录会自动保留。",
						result.CurrentVersion, result.CurrentModels, result.RemoteVersion, result.RemoteModels,
						result.Added, result.Updated, result.Retired, result.Removed,
					))
					installButton.Enable()
				} else {
					title.SetText("模型目录已是最新")
					detail.SetText(fmt.Sprintf("当前版本 %s，共 %d 个内置模型。", result.CurrentVersion, result.CurrentModels))
				}
			})
		}()
	}
	checkButton.OnTapped = check
	installButton.OnTapped = func() {
		installButton.Disable()
		checkButton.Disable()
		title.SetText("正在安装模型目录…")
		go func() {
			info, err := controller.ApplyModelCatalog(checked)
			fyne.Do(func() {
				checkButton.Enable()
				if err != nil {
					title.SetText("模型目录安装失败")
					detail.SetText(err.Error())
					installButton.Enable()
					return
				}
				title.SetText("模型目录更新完成")
				message := fmt.Sprintf("已安装 %s，共 %d 个模型。", info.CatalogVersion, info.ModelCount)
				if controller.Snapshot().Status == desktop.StatusRunning {
					message += "\n重启内核后生效。"
				} else {
					message += "\n下次启动内核时生效。"
				}
				detail.SetText(message)
			})
		}()
	}
	updateDialog.Show()
	check()
}

func showProviderHelp(window fyne.Window) {
	rows := container.NewVBox()
	for _, item := range providercatalog.All() {
		provider := item
		name := widget.NewLabelWithStyle(provider.DisplayName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		keyButton := newPointerButton("创建 / 管理密钥", func() {
			if err := desktop.Open(provider.CredentialURL); err != nil {
				dialog.ShowError(err, window)
			}
		})
		docsButton := newPointerButton("官方文档", func() {
			if err := desktop.Open(provider.DocsURL); err != nil {
				dialog.ShowError(err, window)
			}
		})
		rows.Add(container.NewBorder(nil, nil, name, container.NewHBox(keyButton, docsButton)))
	}
	scroll := container.NewVScroll(rows)
	scroll.SetMinSize(fyne.NewSize(560, 330))
	helpDialog := dialog.NewCustomWithoutButtons("获取厂商密钥", scroll, window)
	closeButton := newPointerButton("关闭", helpDialog.Hide)
	helpDialog.SetButtons([]fyne.CanvasObject{closeButton})
	helpDialog.Show()
}

func formatBytes(value int64) string {
	if value <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}

func formatETA(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	return fmt.Sprintf(" · 预计 %s", (time.Duration(seconds) * time.Second).Round(time.Second))
}

func installStageLabel(stage string) string {
	switch stage {
	case "connecting":
		return "正在连接"
	case "downloading":
		return "正在下载"
	case "retrying":
		return "正在重试"
	case "extracting":
		return "正在解压"
	case "verifying":
		return "正在校验"
	case "canceling":
		return "正在取消"
	default:
		return "等待下载"
	}
}

func conciseVersion(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		if strings.EqualFold(field, "version") && index+1 < len(fields) {
			return fields[index+1]
		}
	}
	return ""
}
