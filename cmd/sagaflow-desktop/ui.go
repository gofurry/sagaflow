package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// pointerButton keeps Fyne's native button rendering while making desktop
// affordance explicit: every enabled action uses the hand cursor.
type pointerButton struct {
	widget.Button
}

func newPointerButton(text string, tapped func()) *pointerButton {
	button := &pointerButton{}
	button.Text = text
	button.OnTapped = tapped
	button.ExtendBaseWidget(button)
	return button
}

func newPointerButtonWithIcon(text string, icon fyne.Resource, tapped func()) *pointerButton {
	button := newPointerButton(text, tapped)
	button.Icon = icon
	return button
}

func (button *pointerButton) CreateRenderer() fyne.WidgetRenderer {
	renderer := button.Button.CreateRenderer()
	button.ExtendBaseWidget(button)
	return renderer
}

func (button *pointerButton) MinSize() fyne.Size {
	button.ExtendBaseWidget(button)
	return button.BaseWidget.MinSize()
}

func (button *pointerButton) Cursor() desktop.Cursor {
	if button.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// preferenceCheck deliberately suppresses Fyne's circular hover/focus
// highlight while retaining the familiar checkbox rendering and behaviour.
type preferenceCheck struct {
	widget.Check
}

func newPreferenceCheck(text string, checked bool, changed func(bool)) *preferenceCheck {
	check := &preferenceCheck{}
	check.Text = text
	check.Checked = checked
	check.OnChanged = changed
	check.ExtendBaseWidget(check)
	return check
}

func (check *preferenceCheck) CreateRenderer() fyne.WidgetRenderer {
	renderer := check.Check.CreateRenderer()
	check.ExtendBaseWidget(check)
	return renderer
}

func (check *preferenceCheck) MinSize() fyne.Size {
	size := check.Check.MinSize()
	check.ExtendBaseWidget(check)
	return size
}

func (check *preferenceCheck) Cursor() desktop.Cursor {
	if check.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

func (check *preferenceCheck) MouseIn(*desktop.MouseEvent)    {}
func (check *preferenceCheck) MouseMoved(*desktop.MouseEvent) {}
func (check *preferenceCheck) MouseOut()                      {}
func (check *preferenceCheck) FocusGained()                   {}
func (check *preferenceCheck) FocusLost()                     {}
