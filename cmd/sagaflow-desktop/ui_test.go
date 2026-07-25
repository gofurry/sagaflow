package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func TestPointerButtonCursorAndDisabledState(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()

	button := newPointerButton("测试按钮", func() {})
	test.WidgetRenderer(button)

	if got := button.Cursor(); got != desktop.PointerCursor {
		t.Fatalf("enabled button cursor = %T, want pointer cursor", got)
	}
	button.Disable()
	if got := button.Cursor(); got != desktop.DefaultCursor {
		t.Fatalf("disabled button cursor = %T, want default cursor", got)
	}
}

func TestPreferenceCheckTogglesWithoutHoverState(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()

	var changed bool
	check := newPreferenceCheck("测试偏好", false, func(value bool) {
		changed = value
	})
	test.WidgetRenderer(check)

	check.MouseIn(nil)
	check.MouseMoved(nil)
	check.MouseOut()
	check.Tapped(&fyne.PointEvent{})

	if !check.Checked || !changed {
		t.Fatalf("preference check did not toggle: checked=%t changed=%t", check.Checked, changed)
	}
	if got := check.Cursor(); got != desktop.PointerCursor {
		t.Fatalf("preference cursor = %T, want pointer cursor", got)
	}
}
