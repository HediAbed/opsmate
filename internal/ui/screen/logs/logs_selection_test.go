package logs

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const (
	wrappedProbeWidth      = 120
	wrappedProbeHeight     = 30
	wrappedProbeLineCount  = 60
	wrappedProbeFillerRuns = 200
	clickedProbeRow        = 4
	dragRowSpan            = 6
)

func newWrappedLogsModel(t *testing.T) LogsModel {
	t.Helper()
	model := newLogsForTest(t)
	model.SetSize(wrappedProbeWidth, wrappedProbeHeight)
	model.selectedPod = "web"
	lines := make([]string, wrappedProbeLineCount)
	for position := range lines {
		lines[position] = fmt.Sprintf("line-%03d ", position) + strings.Repeat("x", wrappedProbeFillerRuns)
	}
	model.allLines = lines
	model.applyFilter()
	model.logView.SetContent(model.colorizeLines(model.filteredLines))
	model.logView.GotoBottom()
	return model
}

func TestWrappedLinesOccupyMoreRowsThanLines(t *testing.T) {
	model := newWrappedLogsModel(t)
	index := model.logRowIndex()
	if index.totalRows <= len(model.filteredLines) {
		t.Fatalf("rendered rows = %d, want more than %d logical lines",
			index.totalRows, len(model.filteredLines))
	}
	if got := index.totalRows; got != model.logView.TotalLineCount() {
		t.Fatalf("row index total = %d, viewport total = %d", got, model.logView.TotalLineCount())
	}
}

func TestInspectionStartsOnAVisibleLineWhenLinesWrap(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()

	index := model.logRowIndex()
	cursorRow := index.rowOf(model.lineCursor)
	top, bottom := model.logView.YOffset(), model.logView.YOffset()+model.logView.Height()
	if cursorRow < top || cursorRow >= bottom {
		t.Fatalf("cursor row %d outside visible rows [%d,%d)", cursorRow, top, bottom)
	}
}

func TestMovingTheInspectCursorDoesNotJumpToTheOldestLines(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()
	offsetBeforeMove := model.logView.YOffset()

	model.moveInspectCursor(1)

	if model.logView.YOffset() < offsetBeforeMove {
		t.Fatalf("offset moved backwards from %d to %d when advancing the cursor",
			offsetBeforeMove, model.logView.YOffset())
	}
	index := model.logRowIndex()
	cursorRow := index.rowOf(model.lineCursor)
	top, bottom := model.logView.YOffset(), model.logView.YOffset()+model.logView.Height()
	if cursorRow < top || cursorRow >= bottom {
		t.Fatalf("cursor row %d outside visible rows [%d,%d) after moving", cursorRow, top, bottom)
	}
}

func TestClickingALogLineSelectsTheLineUnderTheCursor(t *testing.T) {
	model := newWrappedLogsModel(t)
	clickedRow := clickedProbeRow
	updated, command := model.handleLogLineClick(tea.MouseClickMsg{
		X:      1,
		Y:      model.logViewportTopRow() + clickedRow,
		Button: tea.MouseLeft,
	})

	if command != nil {
		t.Fatalf("click returned command %v, want none", command)
	}
	if !updated.inspectMode {
		t.Fatal("click did not enter inspect mode")
	}
	wantLine := model.logRowIndex().lineAt(model.logView.YOffset() + clickedRow)
	if updated.lineCursor != wantLine {
		t.Fatalf("selected line = %d, want %d", updated.lineCursor, wantLine)
	}
}

func TestClickOutsideTheLogViewportIsIgnored(t *testing.T) {
	model := newWrappedLogsModel(t)
	updated, _ := model.handleLogLineClick(tea.MouseClickMsg{X: 1, Y: 0, Button: tea.MouseLeft})
	if updated.inspectMode {
		t.Fatal("click above the viewport selected a line")
	}
	updated, _ = model.handleLogLineClick(tea.MouseClickMsg{X: 1, Y: wrappedProbeHeight, Button: tea.MouseLeft})
	if updated.inspectMode {
		t.Fatal("click below the viewport selected a line")
	}
	updated, _ = model.handleLogLineClick(tea.MouseClickMsg{
		X:      1,
		Y:      model.logViewportTopRow(),
		Button: tea.MouseRight,
	})
	if updated.inspectMode {
		t.Fatal("right click selected a line")
	}
}

func TestCopyingTheSelectedLineUsesThatLineOnly(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()
	updated, command := model.copySelectedLine()
	if command == nil || updated.statusMsg == "" {
		t.Fatalf("copy selected line = command:%v status:%q", command != nil, updated.statusMsg)
	}
}

func TestCopyingWithoutASelectedLineDoesNothing(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.lineCursor = len(model.filteredLines)
	updated, command := model.copySelectedLine()
	if command != nil || updated.statusMsg != "" {
		t.Fatalf("out of range copy = command:%v status:%q", command != nil, updated.statusMsg)
	}
}

func TestInspectCopyKeyCopiesTheSelectedLine(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()
	updated, command := model.handleInspectKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if command == nil || updated.statusMsg == "" {
		t.Fatalf("inspect copy = command:%v status:%q", command != nil, updated.statusMsg)
	}
}

func TestRowIndexMatchesTheViewportAtEveryWrapBoundary(t *testing.T) {
	for _, widthOffset := range []int{-2, -1, 0, 1, 2} {
		model := newLogsForTest(t)
		model.SetSize(wrappedProbeWidth, wrappedProbeHeight)
		line := strings.Repeat("a", model.logView.Width()+widthOffset)
		model.allLines = []string{line, line, line}
		model.applyFilter()
		model.logView.SetContent(model.colorizeLines(model.filteredLines))

		if got, want := model.logRowIndex().totalRows, model.logView.TotalLineCount(); got != want {
			t.Errorf("line width %+d from the viewport width: index rows = %d, viewport rows = %d",
				widthOffset, got, want)
		}
	}
}

func TestRowIndexAccountsForTheInspectCursorPrefix(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()
	if got, want := model.logRowIndex().totalRows, model.logView.TotalLineCount(); got != want {
		t.Fatalf("index rows = %d, viewport rows = %d while inspecting", got, want)
	}
}

func TestClickingWhileTheContainerPopupIsOpenChangesNothing(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.showContainerPopup = true
	updated, command := model.updateLogInputMessage(tea.MouseClickMsg{
		X:      1,
		Y:      model.logViewportTopRow() + clickedProbeRow,
		Button: tea.MouseLeft,
	})
	if command != nil || updated.inspectMode || updated.paused {
		t.Fatalf("container popup click = command:%v inspect:%v paused:%v",
			command != nil, updated.inspectMode, updated.paused)
	}
}

func TestDraggingSelectsARangeOfLines(t *testing.T) {
	model := newWrappedLogsModel(t)
	firstRow := model.logViewportTopRow() + clickedProbeRow
	pressed, _ := model.handleLogLineClick(tea.MouseClickMsg{X: 1, Y: firstRow, Button: tea.MouseLeft})
	dragged, command := pressed.handleLogLineDrag(tea.MouseMotionMsg{X: 1, Y: firstRow + dragRowSpan, Button: tea.MouseLeft})

	if command != nil {
		t.Fatalf("drag returned command %v, want none", command)
	}
	first, last := dragged.selectionRange()
	if last <= first {
		t.Fatalf("selection range = [%d,%d], want more than one line", first, last)
	}
	if got := len(dragged.selectedLines()); got != last-first+1 {
		t.Fatalf("selected lines = %d, want %d", got, last-first+1)
	}
}

func TestDragWithoutAPressIsIgnored(t *testing.T) {
	model := newWrappedLogsModel(t)
	updated, _ := model.handleLogLineDrag(tea.MouseMotionMsg{X: 1, Y: model.logViewportTopRow(), Button: tea.MouseLeft})
	if updated.inspectMode {
		t.Fatal("drag without a press started a selection")
	}
}

func TestDragOutsideTheViewportKeepsTheSelection(t *testing.T) {
	model := newWrappedLogsModel(t)
	pressed, _ := model.handleLogLineClick(tea.MouseClickMsg{
		X: 1, Y: model.logViewportTopRow() + clickedProbeRow, Button: tea.MouseLeft,
	})
	dragged, _ := pressed.handleLogLineDrag(tea.MouseMotionMsg{X: 1, Y: 0, Button: tea.MouseLeft})
	if dragged.lineCursor != pressed.lineCursor {
		t.Fatalf("cursor moved to %d on an out-of-bounds drag, want %d", dragged.lineCursor, pressed.lineCursor)
	}
}

func TestReleasingEndsTheDragButKeepsTheSelection(t *testing.T) {
	model := newWrappedLogsModel(t)
	pressed, _ := model.handleLogLineClick(tea.MouseClickMsg{
		X: 1, Y: model.logViewportTopRow() + clickedProbeRow, Button: tea.MouseLeft,
	})
	released, command := pressed.handleLogSelectionRelease()
	if command != nil || released.selectingWithMouse {
		t.Fatalf("release = command:%v selecting:%v", command != nil, released.selectingWithMouse)
	}
	if !released.inspectMode || released.lineCursor != pressed.lineCursor {
		t.Fatal("release discarded the selection")
	}
}

func TestExitingInspectClearsTheSelection(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.startLogInspection()
	exited, _ := model.handleInspectKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if exited.inspectMode || exited.selectionAnchor != noLineSelected {
		t.Fatalf("exit = inspect:%v anchor:%d", exited.inspectMode, exited.selectionAnchor)
	}
}

func TestMouseRoutingReachesDragAndRelease(t *testing.T) {
	model := newWrappedLogsModel(t)
	pressRow := model.logViewportTopRow() + clickedProbeRow
	pressed, _ := model.updateLogInputMessage(tea.MouseClickMsg{X: 1, Y: pressRow, Button: tea.MouseLeft})
	dragged, _ := pressed.updateLogInputMessage(tea.MouseMotionMsg{X: 1, Y: pressRow + dragRowSpan, Button: tea.MouseLeft})
	if first, last := dragged.selectionRange(); last <= first {
		t.Fatalf("routed drag produced range [%d,%d]", first, last)
	}
	released, _ := dragged.updateLogInputMessage(tea.MouseReleaseMsg{X: 1, Y: pressRow, Button: tea.MouseLeft})
	if released.selectingWithMouse {
		t.Fatal("routed release did not end the drag")
	}
}

func TestSelectionWithoutAnAnchorIsJustTheCursor(t *testing.T) {
	model := newWrappedLogsModel(t)
	model.inspectMode = true
	model.lineCursor = 2
	model.selectionAnchor = len(model.filteredLines)
	first, last := model.selectionRange()
	if first != model.lineCursor || last != model.lineCursor {
		t.Fatalf("range = [%d,%d], want the cursor only", first, last)
	}
}
