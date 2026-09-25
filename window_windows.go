package main

// The settings window on Windows, built from the system's own controls, and the
// icon in the notification area with its menu. It says and does what the macOS
// window does.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pAdjustWindowRectExForDpi   = user32.NewProc("AdjustWindowRectExForDpi")
	pAllowSetForegroundWindow   = user32.NewProc("AllowSetForegroundWindow")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pDrawTextW                  = user32.NewProc("DrawTextW")
	pEnableWindow               = user32.NewProc("EnableWindow")
	pFillRect                   = user32.NewProc("FillRect")
	pFindWindowW                = user32.NewProc("FindWindowW")
	pGetClientRect              = user32.NewProc("GetClientRect")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pGetDC                      = user32.NewProc("GetDC")
	pGetDpiForWindow            = user32.NewProc("GetDpiForWindow")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pGetSysColor                = user32.NewProc("GetSysColor")
	pGetSysColorBrush           = user32.NewProc("GetSysColorBrush")
	pGetWindowTextLengthW       = user32.NewProc("GetWindowTextLengthW")
	pGetWindowTextW             = user32.NewProc("GetWindowTextW")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pIsDialogMessageW           = user32.NewProc("IsDialogMessageW")
	pIsIconic                   = user32.NewProc("IsIconic")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pMoveWindow                 = user32.NewProc("MoveWindow")
	pPostMessageW               = user32.NewProc("PostMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pRegisterWindowMessageW     = user32.NewProc("RegisterWindowMessageW")
	pReleaseDC                  = user32.NewProc("ReleaseDC")
	pSendMessageW               = user32.NewProc("SendMessageW")
	pSetFocus                   = user32.NewProc("SetFocus")
	pGetFocus                   = user32.NewProc("GetFocus")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pSetWindowTextW             = user32.NewProc("SetWindowTextW")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pSystemParametersInfoForDpi = user32.NewProc("SystemParametersInfoForDpi")
	pSystemParametersInfoW      = user32.NewProc("SystemParametersInfoW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pTranslateMessage           = user32.NewProc("TranslateMessage")

	pCreateFontIndirectW   = gdi32.NewProc("CreateFontIndirectW")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	pSelectObject          = gdi32.NewProc("SelectObject")
	pSetBkColor            = gdi32.NewProc("SetBkColor")
	pSetBkMode             = gdi32.NewProc("SetBkMode")
	pSetTextColor          = gdi32.NewProc("SetTextColor")

	pSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	pShellNotifyIconW            = shell32.NewProc("Shell_NotifyIconW")

	pLoadIconMetric = comctl32.NewProc("LoadIconMetric")
	pTaskDialog     = comctl32.NewProc("TaskDialog")

	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
)

const (
	windowClass = "CCTokenManagerWindow"
	windowTitle = "CC Token Manager"

	// No maximise box: the window is resized by its edges, never blown up to
	// the whole screen.
	mainStyle   = wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsClipChildren
	mainExStyle = wsExControlParent

	wmTray = 0x8001 // the icon in the notification area was clicked
	wmDone = 0x8002 // work done off the window's thread has an answer
	wmShow = 0x8003 // a second copy of the app asks for the window
)

const (
	idList = 100 + iota
	idAdd
	idRemove
	idName
	idToken
	idTest
	idDir
	idChoose
	idTheme
	idSave
	idApply
)

const (
	wsCaption         = 0x00C00000
	wsSysMenu         = 0x00080000
	wsThickFrame      = 0x00040000
	wsMinimizeBox     = 0x00020000
	wsClipChildren    = 0x02000000
	wsChild           = 0x40000000
	wsVisible         = 0x10000000
	wsTabStop         = 0x00010000
	wsVScroll         = 0x00200000
	wsExClientEdge    = 0x00000200
	wsExControlParent = 0x00010000

	esMultiline         = 0x0004
	esAutoVScroll       = 0x0040
	esAutoHScroll       = 0x0080
	bsDefPushButton     = 0x0001
	ssRight             = 0x0002
	lbsNotify           = 0x0001
	lbsOwnerDrawFixed   = 0x0010
	lbsHasStrings       = 0x0040
	lbsNoIntegralHeight = 0x0100
	cbsDropDownList     = 0x0003

	wmCreate          = 0x0001
	wmDestroy         = 0x0002
	wmSize            = 0x0005
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmGetMinMaxInfo   = 0x0024
	wmDrawItem        = 0x002B
	wmMeasureItem     = 0x002C
	wmSetFont         = 0x0030
	wmCommand         = 0x0111
	wmCtlColorStatic  = 0x0138
	wmLButtonUp       = 0x0202
	wmRButtonUp       = 0x0205
	wmDpiChanged      = 0x02E0
	dmGetDefID        = 0x0400

	lbAddString     = 0x0180
	lbDeleteString  = 0x0182
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbSetItemHeight = 0x01A0
	lbnSelChange    = 1
	cbAddString     = 0x0143
	cbGetCurSel     = 0x0147
	cbSetCurSel     = 0x014E
	enChange        = 0x0300
	enSetFocus      = 0x0100
	enKillFocus     = 0x0200
	emSetSel        = 0x00B1
	emSetCueBanner  = 0x1501

	colorWindow        = 5
	colorWindowText    = 8
	colorHighlight     = 13
	colorHighlightText = 14
	colorGrayText      = 17

	dtCenter      = 0x0001
	dtVCenter     = 0x0004
	dtSingleLine  = 0x0020
	dtNoPrefix    = 0x0800
	dtEndEllipsis = 0x8000
	odsSelected   = 0x0001

	nimAdd    = 0
	nimDelete = 2
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size, Style         uint32
	WndProc             uintptr
	ClsExtra, WndExtra  int32
	Instance, Icon      uintptr
	Cursor, Background  uintptr
	MenuName, ClassName *uint16
	IconSm              uintptr
}

type minMaxInfo struct{ Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize point }

type drawItemStruct struct {
	CtlType, CtlID, ItemID, ItemAction, ItemState uint32
	HwndItem, HDC                                 uintptr
	RcItem                                        rect
	ItemData                                      uintptr
}

type measureItemStruct struct {
	CtlType, CtlID, ItemID, ItemWidth, ItemHeight uint32
	ItemData                                      uintptr
}

type logFont struct {
	Height, Width, Escapement, Orientation, Weight int32
	Italic, Underline, StrikeOut, CharSet          byte
	OutPrecision, ClipPrecision, Quality, Pitch    byte
	FaceName                                       [32]uint16
}

type nonClientMetrics struct {
	Size                                                        uint32
	BorderWidth, ScrollWidth, ScrollHeight, CapWidth, CapHeight int32
	CaptionFont                                                 logFont
	SmCapWidth, SmCapHeight                                     int32
	SmCaptionFont                                               logFont
	MenuWidth, MenuHeight                                       int32
	MenuFont, StatusFont, MessageFont                           logFont
	PaddedBorderWidth                                           int32
}

type notifyIconData struct {
	Size             uint32
	Hwnd             uintptr
	ID, Flags        uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State, StateMask uint32
	Info             [256]uint16
	Version          uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             [16]byte
	BalloonIcon      uintptr
}

type guid struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

var ui struct {
	hwnd                                                                  uintptr
	list, add, remove, name, token, test, dir, choose, theme, save, apply uintptr
	labels                                                                [4]uintptr
	font, mono                                                            uintptr // the system's message font, and a monospaced one for the fields
	dpi                                                                   uintptr
	taskbarCreated                                                        uintptr
	profiles                                                              []Profile
	shown                                                                 int    // the profile in the form
	applied                                                               string // the profile with the tick, applied right now
	filling                                                               bool   // the form is being filled in, not typed into
	secret                                                                string // the token the field stands for while it shows noise
}

// Work done off the window's thread hands its answer back through pending.
var (
	pendingMu sync.Mutex
	pending   []func()
)

// The window's thread is the one that made it, and the only one to touch it.
func init() { runtime.LockOSThread() }

// runWindow opens the settings window on the active profile, with the tick on
// it if applied is true, and says status first if there is anything to say. It
// returns when the app quits.
func runWindow(s Settings, applied bool, status string) {
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED, for the folder dialog
	ui.profiles = append([]Profile(nil), s.Profiles...)
	active := s.Active().ID
	for i, p := range ui.profiles {
		if p.ID == active {
			ui.shown = i
		}
	}
	if applied {
		ui.applied = active
	}
	// The first launch adds the app to the programs Windows starts at login, and
	// only it: a removal in Settings or Task Manager then stays.
	if !config.LaunchAtLogin && registerAtLogin() {
		config.LaunchAtLogin = true
		saveConfig(config)
	}

	inst, _, _ := pGetModuleHandleW.Call(0)
	wc := wndClassEx{
		Style:      3, // CS_HREDRAW | CS_VREDRAW
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   inst,
		Background: colorWindow + 1,
		ClassName:  u16(windowClass),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.Cursor, _, _ = pLoadCursorW.Call(0, 32512)                         // IDC_ARROW
	pLoadIconMetric.Call(inst, 1, 1, uintptr(unsafe.Pointer(&wc.Icon)))   // the app's icon, large
	pLoadIconMetric.Call(inst, 1, 0, uintptr(unsafe.Pointer(&wc.IconSm))) // and small
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	ui.taskbarCreated, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	pCreateWindowExW.Call(mainExStyle, uintptr(unsafe.Pointer(u16(windowClass))), uintptr(unsafe.Pointer(u16(windowTitle))),
		mainStyle, 0x80000000, 0x80000000, 0, 0, 0, 0, inst, 0) // CW_USEDEFAULT; WM_CREATE fills it in

	// As large as its contents need, and in the middle of the screen.
	w, h := minWindow()
	var work rect
	pSystemParametersInfoW.Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0) // SPI_GETWORKAREA
	pSetWindowPos.Call(ui.hwnd, 0, uintptr(work.Left+(work.Right-work.Left-w)/2),
		uintptr(work.Top+(work.Bottom-work.Top-h)/2), uintptr(w), uintptr(h), 0x4) // SWP_NOZORDER
	tray(nimAdd)
	if status != "" {
		onWindow(func() { say(status, false) })
	}

	var m msg
	for {
		if r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		if d, _, _ := pIsDialogMessageW.Call(ui.hwnd, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue // Tab, Return and the like, handled as in a dialog
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	if m == ui.taskbarCreated && m != 0 {
		tray(nimAdd) // Explorer restarted and forgot the icon
		return 0
	}
	switch m {
	case wmCreate:
		ui.hwnd = hwnd
		build()
		return 0
	case wmSize:
		layout()
		return 0
	case wmGetMinMaxInfo:
		if ui.font != 0 { // it comes before WM_CREATE too
			w, h := minWindow()
			at[minMaxInfo](lp).MinTrackSize = point{w, h}
		}
		return 0
	case wmDpiChanged:
		ui.dpi = wp >> 16 & 0xFFFF
		fonts()
		r := at[rect](lp)
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x14) // SWP_NOZORDER | SWP_NOACTIVATE
		return 0
	case wmMeasureItem:
		at[measureItemStruct](lp).ItemHeight = uint32(rowHeight())
		return 1
	case wmDrawItem:
		drawRow(at[drawItemStruct](lp))
		return 1
	case wmCtlColorStatic:
		// A label is quieter than what it labels.
		c, _, _ := pGetSysColor.Call(colorGrayText)
		pSetTextColor.Call(wp, c)
		c, _, _ = pGetSysColor.Call(colorWindow)
		pSetBkColor.Call(wp, c)
		b, _, _ := pGetSysColorBrush.Call(colorWindow)
		return b
	case wmCommand:
		command(wp&0xFFFF, wp>>16&0xFFFF)
		return 0
	case dmGetDefID:
		return 0x534B<<16 | idApply // DC_HASDEFID: Return presses Apply profile
	case wmClose:
		pShowWindow.Call(hwnd, 0) // SW_HIDE: the app stays in the notification area
		return 0
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wp != 0 {
			quit() // logging off or shutting down: the profile goes as on Quit
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	case wmTray:
		switch lp & 0xFFFF {
		case wmLButtonUp:
			showWindow()
		case wmRButtonUp:
			trayMenu()
		}
		return 0
	case wmDone:
		pendingMu.Lock()
		jobs := pending
		pending = nil
		pendingMu.Unlock()
		for _, f := range jobs {
			f()
		}
		return 0
	case wmShow:
		showWindow()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// build makes the window's controls: the list of profiles with the buttons
// that add and remove one under it, and beside it the form for the profile
// selected, with Save and Apply profile in its bottom corner.
func build() {
	ui.dpi, _, _ = pGetDpiForWindow.Call(ui.hwnd)
	const (
		edit   = wsChild | wsVisible | wsTabStop | esAutoHScroll
		button = wsChild | wsVisible | wsTabStop
		label  = wsChild | wsVisible | ssRight
	)
	ui.list = child("LISTBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|lbsNotify|lbsOwnerDrawFixed|lbsHasStrings|lbsNoIntegralHeight, wsExClientEdge, idList)
	ui.add = child("BUTTON", "+", button, 0, idAdd)
	ui.remove = child("BUTTON", "−", button, 0, idRemove)
	ui.labels[0] = child("STATIC", "Profile name", label, 0, 0)
	ui.name = child("EDIT", "", edit, wsExClientEdge, idName)
	ui.labels[1] = child("STATIC", "OAuth token", label, 0, 0)
	ui.token = child("EDIT", "", wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll, wsExClientEdge, idToken)
	ui.test = child("BUTTON", "Test connection", button, 0, idTest)
	ui.labels[2] = child("STATIC", "Profile folder", label, 0, 0)
	ui.dir = child("EDIT", "", edit, wsExClientEdge, idDir)
	ui.choose = child("BUTTON", "Choose…", button, 0, idChoose)
	ui.labels[3] = child("STATIC", "Profile theme", label, 0, 0)
	ui.theme = child("COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropDownList, 0, idTheme)
	ui.save = child("BUTTON", "Save", button, 0, idSave)
	ui.apply = child("BUTTON", "Apply profile", button|bsDefPushButton, 0, idApply)
	fonts()
	for _, p := range ui.profiles {
		pSendMessageW.Call(ui.list, lbAddString, 0, uintptr(unsafe.Pointer(u16(p.Name))))
	}
	for _, t := range themes {
		pSendMessageW.Call(ui.theme, cbAddString, 0, uintptr(unsafe.Pointer(u16(t.Name))))
	}
	show(ui.shown)
}

func child(class, text string, style, exStyle, id uintptr) uintptr {
	inst, _, _ := pGetModuleHandleW.Call(0)
	h, _, _ := pCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(text))),
		style, 0, 0, 0, 0, ui.hwnd, id, inst, 0)
	return h
}

// fonts takes the system's message font at the window's DPI, and Consolas at
// its size for the fields, and lays the window out again in them.
func fonts() {
	var ncm nonClientMetrics
	ncm.Size = uint32(unsafe.Sizeof(ncm))
	pSystemParametersInfoForDpi.Call(0x29, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0, ui.dpi) // SPI_GETNONCLIENTMETRICS
	mono := ncm.MessageFont
	copy(mono.FaceName[:], wide("Consolas"))
	old := []uintptr{ui.font, ui.mono}
	ui.font, _, _ = pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&ncm.MessageFont)))
	ui.mono, _, _ = pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&mono)))
	for _, h := range append(ui.labels[:], ui.list, ui.add, ui.remove, ui.test, ui.choose, ui.theme, ui.save, ui.apply) {
		pSendMessageW.Call(h, wmSetFont, ui.font, 1)
	}
	for _, h := range []uintptr{ui.name, ui.token, ui.dir} {
		pSendMessageW.Call(h, wmSetFont, ui.mono, 1)
	}
	for _, f := range old {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	pSendMessageW.Call(ui.list, lbSetItemHeight, 0, uintptr(rowHeight()))
	layout()
}

// sizes are the window's measurements at its DPI, in its fonts.
type sizes struct {
	margin, gap, line, editH, buttonH, tokenH, tokenMinW, labelW, themeH int32
	testW, chooseW, saveW, applyW                                        int32
}

func measure() sizes {
	var s sizes
	s.margin, s.gap = px(12), px(8)
	_, s.line = extent(ui.font, "Ay")
	ch, mono := extent(ui.mono, "0")
	s.editH = mono + px(8)
	s.buttonH = s.line + px(12)
	s.tokenH = 4*mono + px(8)    // four lines of the token
	s.tokenMinW = 48*ch + px(28) // and 48 characters of it, beside the scroll bar
	for _, t := range []string{"Profile name", "OAuth token", "Profile folder", "Profile theme"} {
		s.labelW = max(s.labelW, first(extent(ui.font, t)))
	}
	// A drop-down list is as tall as its font makes it, whatever it is asked to
	// be, and the list that drops from it as tall as its items.
	var box rect
	pGetClientRect.Call(ui.theme, uintptr(unsafe.Pointer(&box)))
	s.themeH = box.Bottom
	button := func(t string) int32 { return max(first(extent(ui.font, t))+px(24), px(80)) }
	s.testW, s.chooseW, s.saveW, s.applyW = button("Test connection"), button("Choose…"), button("Save"), button("Apply profile")
	return s
}

func layout() {
	if ui.font == 0 {
		return
	}
	var rc rect
	pGetClientRect.Call(ui.hwnd, uintptr(unsafe.Pointer(&rc)))
	s := measure()
	w, h := rc.Right, rc.Bottom
	// The list keeps three tenths of the width, the form takes the rest.
	listW := (w - 2*s.margin) * 3 / 10
	listH := h - 2*s.margin - s.gap - s.buttonH
	put(ui.list, s.margin, s.margin, listW, listH)
	put(ui.add, s.margin, s.margin+listH+s.gap, s.buttonH, s.buttonH)
	put(ui.remove, s.margin+s.buttonH, s.margin+listH+s.gap, s.buttonH, s.buttonH)

	x := s.margin + listW + 2*s.gap
	cx := x + s.labelW + s.gap
	cw := w - s.margin - cx
	y := s.margin
	put(ui.labels[0], x, y+(s.editH-s.line)/2, s.labelW, s.line)
	put(ui.name, cx, y, cw, s.editH)
	y += s.editH + s.gap
	put(ui.labels[1], x, y+(s.editH-s.line)/2, s.labelW, s.line) // on the token's first line
	put(ui.token, cx, y, cw, s.tokenH)
	y += s.tokenH + s.gap
	put(ui.test, cx, y, s.testW, s.buttonH)
	y += s.buttonH + s.gap
	put(ui.labels[2], x, y+(s.buttonH-s.line)/2, s.labelW, s.line)
	put(ui.dir, cx, y+(s.buttonH-s.editH)/2, cw-s.chooseW-s.gap, s.editH)
	put(ui.choose, cx+cw-s.chooseW, y, s.chooseW, s.buttonH)
	y += s.buttonH + s.gap
	put(ui.labels[3], x, y+(s.themeH-s.line)/2, s.labelW, s.line)
	put(ui.theme, cx, y, cw, s.themeH)
	by := h - s.margin - s.buttonH
	put(ui.apply, w-s.margin-s.applyW, by, s.applyW, s.buttonH)
	put(ui.save, w-s.margin-s.applyW-s.gap-s.saveW, by, s.saveW, s.buttonH)
}

// minWindow is the smallest the window may be: room for the form beside a list
// three tenths wide, and for the buttons well clear under it.
func minWindow() (int32, int32) {
	s := measure()
	formW := s.labelW + s.gap + s.tokenMinW
	formH := s.editH + s.gap + s.tokenH + s.gap + s.buttonH + s.gap + s.buttonH + s.gap + s.themeH
	r := rect{0, 0, (formW+2*s.gap)*10/7 + 2*s.margin, 2*s.margin + formH + 4*s.gap + s.buttonH}
	pAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&r)), mainStyle, 0, mainExStyle, ui.dpi)
	return r.Right - r.Left, r.Bottom - r.Top
}

func rowHeight() int32 {
	_, h := extent(ui.font, "Ay")
	return h + px(10)
}

// drawRow draws a profile in the list: a tick when it is the applied profile,
// the way a checked menu item has one, and its name, cut short with an ellipsis
// rather than run off the edge.
func drawRow(d *drawItemStruct) {
	if d.CtlID != idList || int(d.ItemID) >= len(ui.profiles) {
		return
	}
	p := ui.profiles[d.ItemID]
	back, fore := uintptr(colorWindow), uintptr(colorWindowText)
	if d.ItemState&odsSelected != 0 {
		back, fore = colorHighlight, colorHighlightText
	}
	b, _, _ := pGetSysColorBrush.Call(back)
	pFillRect.Call(d.HDC, uintptr(unsafe.Pointer(&d.RcItem)), b)
	c, _, _ := pGetSysColor.Call(fore)
	pSetTextColor.Call(d.HDC, c)
	pSetBkMode.Call(d.HDC, 1) // TRANSPARENT
	old, _, _ := pSelectObject.Call(d.HDC, ui.font)
	// The mark comes before what it marks, and its place is kept whether or not
	// it is shown, so the names stay in a column.
	tick := d.RcItem
	tick.Left += px(6)
	tick.Right = tick.Left + px(16)
	if p.ID != "" && p.ID == ui.applied {
		drawText(d.HDC, "✓", &tick, dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	}
	name := d.RcItem
	name.Left, name.Right = tick.Right+px(4), name.Right-px(6)
	n := p.Name
	if n == "" {
		n = "Untitled"
	}
	drawText(d.HDC, n, &name, dtVCenter|dtSingleLine|dtNoPrefix|dtEndEllipsis)
	pSelectObject.Call(d.HDC, old)
}

func command(id, code uintptr) {
	switch id {
	case idList:
		if code == lbnSelChange {
			keep()
			if i, _, _ := pSendMessageW.Call(ui.list, lbGetCurSel, 0, 0); int32(i) >= 0 {
				show(int(i))
			}
		}
	case idName:
		// The list, and the default folder, follow the name as it is typed.
		if code == enChange && !ui.filling {
			keep()
			pInvalidateRect.Call(ui.list, 0, 1)
			hint()
		}
	case idToken:
		// Out of focus the field shows noise in place of the token.
		switch code {
		case enSetFocus:
			setText(ui.token, ui.secret)
		case enKillFocus:
			ui.secret = text(ui.token)
			setText(ui.token, noise(ui.secret))
		}
	case idAdd:
		keep()
		b := make([]byte, 6)
		rand.Read(b)
		ui.profiles = append(ui.profiles, Profile{ID: hex.EncodeToString(b), Name: "New profile", Theme: defaultTheme})
		pSendMessageW.Call(ui.list, lbAddString, 0, uintptr(unsafe.Pointer(u16("New profile"))))
		show(len(ui.profiles) - 1)
		pSetFocus.Call(ui.name)
		pSendMessageW.Call(ui.name, emSetSel, 0, ^uintptr(0))
	case idRemove:
		i := ui.shown
		if !ask(fmt.Sprintf("Delete profile “%s”?", text(ui.name)), "Its token is removed from this app.") {
			return
		}
		// The tick cannot stay on a profile that is gone, nor move to one that
		// is not applied.
		if ui.profiles[i].ID == ui.applied {
			ui.applied = ""
		}
		ui.profiles = append(ui.profiles[:i], ui.profiles[i+1:]...)
		pSendMessageW.Call(ui.list, lbDeleteString, uintptr(i), 0)
		show(min(i, len(ui.profiles)-1))
	case idSave:
		keep()
		say(saveProfiles(append([]Profile(nil), ui.profiles...), ui.applied))
	case idApply:
		// Applying switches Claude Code to the profile as it stands on screen,
		// on a thread of its own, with the button off until the answer.
		keep()
		id := ui.profiles[ui.shown].ID
		profiles := append([]Profile(nil), ui.profiles...)
		pEnableWindow.Call(ui.apply, 0)
		go func() {
			msg, ok := applyProfile(profiles, id)
			onWindow(func() {
				pEnableWindow.Call(ui.apply, 1)
				// A profile that did not go through leaves none applied:
				// ~/.claude is back, and no row gets the tick.
				ui.applied = ""
				if ok {
					ui.applied = id
				}
				pInvalidateRect.Call(ui.list, 0, 1)
				say(msg, ok)
			})
		}()
	case idTest:
		// The token is read from the form, not from Credential Manager: it may
		// not be saved.
		token := token()
		pEnableWindow.Call(ui.test, 0)
		go func() {
			msg, ok := testToken(token)
			onWindow(func() {
				pEnableWindow.Call(ui.test, 1)
				say(msg, ok)
			})
		}()
	case idChoose:
		// The dialog opens where the profile's folder is, or would be, or at
		// its nearest parent that exists.
		start := Profile{ID: ui.profiles[ui.shown].ID, Name: text(ui.name), Dir: text(ui.dir)}.Folder()
		for len(start) > 3 {
			if _, err := os.Stat(start); err == nil {
				break
			}
			start = filepath.Dir(start)
		}
		if dir, ok := pickFolder(start); ok {
			setText(ui.dir, dir)
		}
	}
}

// keep puts what the form holds back into the list of profiles.
func keep() {
	p := &ui.profiles[ui.shown]
	p.Name, p.Token, p.Dir = text(ui.name), token(), text(ui.dir)
	i, _, _ := pSendMessageW.Call(ui.theme, cbGetCurSel, 0, 0)
	p.Theme = themes[i].Value
}

// token is the token in the form: what the field holds while in focus, and
// what it stands for while it shows noise.
func token() string {
	if f, _, _ := pGetFocus.Call(); f == ui.token {
		return text(ui.token)
	}
	return ui.secret
}

// noise is a shade for each character of token, shown in its place while the
// field is out of focus: an empty field stays empty. The medium shade: an edit
// control draws Consolas' light one as nothing at all.
func noise(token string) string {
	return strings.Repeat("▒", utf8.RuneCountInString(token))
}

// show fills the form with profile i.
func show(i int) {
	ui.filling = true
	ui.shown = i
	pSendMessageW.Call(ui.list, lbSetCurSel, uintptr(i), 0)
	p := ui.profiles[i]
	setText(ui.name, p.Name)
	ui.secret = p.Token
	if f, _, _ := pGetFocus.Call(); f == ui.token {
		setText(ui.token, p.Token)
	} else {
		setText(ui.token, noise(p.Token))
	}
	setText(ui.dir, p.Dir)
	for n, t := range themes {
		if t.Value == p.Theme {
			pSendMessageW.Call(ui.theme, cbSetCurSel, uintptr(n), 0)
		}
	}
	ui.filling = false
	hint()
	enable := uintptr(0)
	if len(ui.profiles) > 1 {
		enable = 1
	}
	pEnableWindow.Call(ui.remove, enable)
}

// hint shows, in the empty folder field, the folder the profile gets when none
// is chosen.
func hint() {
	p := Profile{ID: ui.profiles[ui.shown].ID, Name: text(ui.name)}
	pSendMessageW.Call(ui.dir, emSetCueBanner, 1, uintptr(unsafe.Pointer(u16(p.Folder()))))
}

func showWindow() {
	if iconic, _, _ := pIsIconic.Call(ui.hwnd); iconic != 0 {
		pShowWindow.Call(ui.hwnd, 9) // SW_RESTORE
	} else {
		pShowWindow.Call(ui.hwnd, 5) // SW_SHOW
	}
	pSetForegroundWindow.Call(ui.hwnd)
}

// tray adds the icon to the notification area, or takes it away.
func tray(op uintptr) {
	nid := notifyIconData{Hwnd: ui.hwnd, ID: 1, Flags: 1 | 2 | 4, CallbackMessage: wmTray} // NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.Size = uint32(unsafe.Sizeof(nid))
	if op == nimAdd {
		inst, _, _ := pGetModuleHandleW.Call(0)
		pLoadIconMetric.Call(inst, 1, 0, uintptr(unsafe.Pointer(&nid.Icon))) // LIM_SMALL
		copy(nid.Tip[:], wide("CC Token Manager"))
	}
	pShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(&nid)))
}

// trayMenu is the icon's menu: Settings… and Quit.
func trayMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	pAppendMenuW.Call(menu, 0, 1, uintptr(unsafe.Pointer(u16("Settings…"))))
	pAppendMenuW.Call(menu, 0x800, 0, 0) // MF_SEPARATOR
	pAppendMenuW.Call(menu, 0, 2, uintptr(unsafe.Pointer(u16("Quit"))))
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(ui.hwnd)                                                                   // or the menu stays open when clicked away from
	cmd, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x80|0x2, uintptr(pt.X), uintptr(pt.Y), 0, ui.hwnd, 0) // TPM_RETURNCMD | TPM_NONOTIFY | TPM_RIGHTBUTTON
	pPostMessageW.Call(ui.hwnd, 0, 0, 0)                                                                 // WM_NULL, or the menu's next opening closes at once
	pDestroyMenu.Call(menu)
	switch cmd {
	case 1:
		showWindow()
	case 2:
		// Quitting takes the profile back and leaves at once, whatever dialog
		// is open: one running its own loop would otherwise hold the app up.
		quit()
		tray(nimDelete)
		os.Exit(0)
	}
}

// onWindow runs f on the window's thread.
func onWindow(f func()) {
	pendingMu.Lock()
	pending = append(pending, f)
	pendingMu.Unlock()
	pPostMessageW.Call(ui.hwnd, wmDone, 0, 0)
}

// say shows an outcome as the macOS window does: the first sentence as the
// headline, the rest as the detail.
func say(text string, ok bool) {
	showWindow()
	head, detail := text, ""
	if i := strings.Index(text, ". "); i >= 0 && len(text) > 70 {
		head, detail = text[:i+1], text[i+2:]
	}
	alert(head, detail, ok)
}

// alert shows a message, over the window once there is one.
func alert(head, detail string, ok bool) {
	icon := uintptr(0xFFFF) // TD_WARNING_ICON
	if ok {
		icon = 0xFFFD // TD_INFORMATION_ICON
	}
	dialog(head, detail, 1, icon) // TDCBF_OK_BUTTON
}

// ask asks yes or no.
func ask(head, detail string) bool {
	return dialog(head, detail, 2|4, 0) == 6 // TDCBF_YES_BUTTON | TDCBF_NO_BUTTON; IDYES
}

func dialog(head, detail string, buttons, icon uintptr) int32 {
	var pressed int32
	pTaskDialog.Call(ui.hwnd, 0, uintptr(unsafe.Pointer(u16("CC Token Manager"))),
		uintptr(unsafe.Pointer(u16(head))), uintptr(unsafe.Pointer(u16(detail))),
		buttons, icon, uintptr(unsafe.Pointer(&pressed)))
	return pressed
}

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog   = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidShellItem        = guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFA}}
)

// pickFolder asks for a folder with the system's own dialog, one that can make
// a new folder, opened at start.
func pickFolder(start string) (string, bool) {
	var dlg unsafe.Pointer
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dlg))); r != 0 {
		return "", false
	}
	defer com(dlg, 2) // Release
	var opts uint32
	com(dlg, 10, uintptr(unsafe.Pointer(&opts))) // GetOptions
	com(dlg, 9, uintptr(opts)|0x20|0x40)         // SetOptions: and FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM
	var folder unsafe.Pointer
	if r, _, _ := pSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(u16(start))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&folder))); r == 0 {
		com(dlg, 12, uintptr(folder)) // SetFolder
		com(folder, 2)
	}
	if com(dlg, 3, ui.hwnd) != 0 { // Show: cancelled, or failed
		return "", false
	}
	var item unsafe.Pointer
	if com(dlg, 20, uintptr(unsafe.Pointer(&item))) != 0 { // GetResult
		return "", false
	}
	defer com(item, 2)
	var name *uint16
	if com(item, 5, 0x80058000, uintptr(unsafe.Pointer(&name))) != 0 { // GetDisplayName(SIGDN_FILESYSPATH)
		return "", false
	}
	defer pCoTaskMemFree.Call(uintptr(unsafe.Pointer(name)))
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(name), 2*n)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(name, n)), true
}

// com calls method n of a COM object: its place in the object's table of
// methods, which starts with IUnknown's three.
func com(obj unsafe.Pointer, n int, args ...uintptr) uintptr {
	method := *(*uintptr)(unsafe.Add(*(*unsafe.Pointer)(obj), n*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func text(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) {
	pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s))))
}

func drawText(dc uintptr, s string, r *rect, flags uintptr) {
	p := wide(s)
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(r)), flags)
}

func put(h uintptr, x, y, w, hgt int32) {
	pMoveWindow.Call(h, uintptr(x), uintptr(y), uintptr(w), uintptr(hgt), 1)
}

// extent is the width and height of s in font f.
func extent(f uintptr, s string) (int32, int32) {
	dc, _, _ := pGetDC.Call(ui.hwnd)
	old, _, _ := pSelectObject.Call(dc, f)
	var sz point
	p := wide(s)
	pGetTextExtentPoint32W.Call(dc, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(&sz)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(ui.hwnd, dc)
	return sz.X, sz.Y
}

func first(a, _ int32) int32 { return a }

// px is a length at the window's DPI, given at 96.
func px(v int32) int32 { return int32((uintptr(v)*ui.dpi + 48) / 96) }

// at reads a pointer the system passes as a number.
func at[T any](p uintptr) *T { return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&p))) }
