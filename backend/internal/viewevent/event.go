// Package viewevent defines the record of a viewer opening a card or report.
package viewevent

import (
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	// CardOpen is the LINE card's button being followed into the dashboard.
	CardOpen Kind = "CARD_OPEN"
	// ReportView is one report page being loaded, from a card or from the web.
	ReportView Kind = "REPORT_VIEW"
	// OverviewView is the executive overview being loaded.
	OverviewView Kind = "OVERVIEW_VIEW"
	// RefreshRequest is the viewer asking for fresh data.
	RefreshRequest Kind = "REFRESH_REQUEST"
)

// Retention is how long events are kept. It matches the audit log.
const Retention = 365 * 24 * time.Hour

// DuplicateWindow collapses a reload or a double tap into one event.
const DuplicateWindow = time.Minute

type Event struct {
	TenantID    uuid.UUID
	RecipientID uuid.UUID
	Kind        Kind
	ReportKey   string
	DeliveryID  *uuid.UUID
	At          time.Time
}

func (kind Kind) Valid() bool {
	switch kind {
	case CardOpen, ReportView, OverviewView, RefreshRequest:
		return true
	}
	return false
}

type Recorder interface {
	Record(Event)
}
