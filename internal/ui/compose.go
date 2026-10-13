package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

const composeHeight = 5

// composeKind says what the composer at the bottom of the detail view posts.
type composeKind int

const (
	composeReply   composeKind = iota // answer to the selected thread
	composeComment                    // new inline comment on a diff line
	composeMessage                    // message on the change itself
)

func (k composeKind) placeholder() string {
	switch k {
	case composeComment:
		return "Write your comment…"
	case composeMessage:
		return "Write a message on the change…"
	}
	return "Write your reply…"
}

func (m detailModel) startReply(text string, unresolved bool) (detailModel, tea.Cmd) {
	t := m.selectedThread()
	if t == nil {
		return m, nil
	}
	m.replyTo = *t
	m.unresolved = unresolved
	return m.openComposer(composeReply, text)
}

func (m detailModel) startNewComment(a lineAnchor) (detailModel, tea.Cmd) {
	m.commentAt = a
	m.unresolved = true
	return m.openComposer(composeComment, "")
}

func (m detailModel) startMessage() (detailModel, tea.Cmd) {
	if m.change == nil {
		return m, nil
	}
	return m.openComposer(composeMessage, "")
}

func (m detailModel) openComposer(kind composeKind, text string) (detailModel, tea.Cmd) {
	if !m.client.Authenticated() {
		m.notice = styleError.Render("posting needs a username and token ")
		return m, nil
	}
	m.composing = true
	m.composeKind = kind
	m.compose.Placeholder = kind.placeholder()
	m.compose.SetValue(text)
	m.compose.Focus()
	m.layout()
	return m, textarea.Blink
}

func (m *detailModel) closeComposer() {
	m.composing = false
	m.compose.Reset()
	m.compose.Blur()
	m.layout()
}

func (m detailModel) updateCompose(msg tea.KeyMsg) (detailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeComposer()
		return m, nil
	case "ctrl+u":
		if m.composeKind != composeMessage {
			m.unresolved = !m.unresolved
		}
		return m, nil
	case "ctrl+s":
		text := strings.TrimSpace(m.compose.Value())
		if text == "" || m.sending {
			return m, nil
		}
		m.sending = true
		switch m.composeKind {
		case composeMessage:
			m.notice = styleYellow.Render("posting message… ")
			return m, m.sendMessage(text)
		case composeComment:
			m.notice = styleYellow.Render("posting comment… ")
			return m, m.sendNewComment(m.commentAt, text, m.unresolved)
		}
		m.notice = styleYellow.Render("posting reply… ")
		return m, m.sendReply(m.replyTo, text, m.unresolved)
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(msg)
	return m, cmd
}

func (m detailModel) sendMessage(text string) tea.Cmd {
	return m.review(gerrit.RevisionCurrent, gerrit.ReviewInput{Message: text})
}

func (m detailModel) sendNewComment(a lineAnchor, text string, unresolved bool) tea.Cmd {
	comment := gerrit.CommentInput{Line: a.line, Side: a.side, Message: text, Unresolved: &unresolved}
	input := gerrit.ReviewInput{Comments: map[string][]gerrit.CommentInput{a.path: {comment}}}
	return m.review(strconv.Itoa(m.change.CurrentRevisionNumber), input)
}

func (m detailModel) sendReply(t thread, text string, unresolved bool) tea.Cmd {
	input := gerrit.ReviewInput{Comments: map[string][]gerrit.CommentInput{t.path: {t.replyInput(text, unresolved)}}}
	return m.review(strconv.Itoa(t.root().PatchSet), input)
}

func (m detailModel) composeView() string {
	state := styleGreen.Render("resolved")
	if m.unresolved {
		state = styleRed.Render("unresolved")
	}
	var header string
	switch m.composeKind {
	case composeMessage:
		header = rule(fmt.Sprintf("Message on change %d", m.change.Number), m.width)
	case composeComment:
		header = rule("New comment on "+m.commentAt.String(), m.width-14) + "  " + state
	default:
		header = rule(fmt.Sprintf("Reply on %s %s", m.replyTo.path, commentLocation(m.replyTo.root())), m.width-14) + "  " + state
	}
	return header + "\n" + m.compose.View()
}
