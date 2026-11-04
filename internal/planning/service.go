package planning

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUnauthorized = errors.New("verified owner required")
	ErrInvalidInput = errors.New("invalid goal input")
	ErrNotFound     = errors.New("goal record not found")
	ErrConflict     = errors.New("goal state does not permit this change")
)

// Goal is future intent, independent of financial events and account balances.
type Goal struct {
	ID             string
	Title          string
	CompletedAt    *time.Time
	ArchivedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Items          []ChecklistItem
	CompletedCount int
	TotalCount     int
}

type ChecklistItem struct {
	ID          string
	Title       string
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Service struct{ repo *repository }

func NewService(pool *pgxpool.Pool) *Service { return &Service{repo: &repository{pool: pool}} }

func (s *Service) ListGoals(ctx context.Context, owner identity.OwnerID) ([]Goal, error) {
	if !owner.Valid() {
		return nil, ErrUnauthorized
	}
	return s.repo.listGoals(ctx, owner, "")
}

func (s *Service) GetGoal(ctx context.Context, owner identity.OwnerID, goalID string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	goals, err := s.repo.listGoals(ctx, owner, goalID)
	if err != nil {
		return Goal{}, err
	}
	if len(goals) == 0 {
		return Goal{}, ErrNotFound
	}
	return goals[0], nil
}

// DeleteGoal is a terminal, naturally retryable change to the owned Goal.
// It neither deletes checklist rows nor creates a financial command/receipt.
func (s *Service) DeleteGoal(ctx context.Context, owner identity.OwnerID, goalID string) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(goalID) {
		return ErrInvalidInput
	}
	return s.repo.deleteGoal(ctx, owner, goalID)
}

func (s *Service) CreateGoal(ctx context.Context, owner identity.OwnerID, title string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	title, err := cleanTitle(title)
	if err != nil {
		return Goal{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Goal{}, err
	}
	if err := s.repo.createGoal(ctx, owner, id, title); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, id)
}

func (s *Service) RenameGoal(ctx context.Context, owner identity.OwnerID, goalID, title string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	title, err := cleanTitle(title)
	if err != nil {
		return Goal{}, err
	}
	if err := s.repo.renameGoal(ctx, owner, goalID, title); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

// SetGoalCompleted requires at least one completed checklist item and no
// incomplete items. Reopening permits further checklist changes.
func (s *Service) SetGoalCompleted(ctx context.Context, owner identity.OwnerID, goalID string, completed bool) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	if err := s.repo.setGoalCompleted(ctx, owner, goalID, completed); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) ArchiveGoal(ctx context.Context, owner identity.OwnerID, goalID string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	if err := s.repo.archiveGoal(ctx, owner, goalID); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) RestoreGoal(ctx context.Context, owner identity.OwnerID, goalID string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	if err := s.repo.restoreGoal(ctx, owner, goalID); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) AddItem(ctx context.Context, owner identity.OwnerID, goalID, title string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) {
		return Goal{}, ErrInvalidInput
	}
	title, err := cleanTitle(title)
	if err != nil {
		return Goal{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Goal{}, err
	}
	if err := s.repo.addItem(ctx, owner, goalID, id, title); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) RenameItem(ctx context.Context, owner identity.OwnerID, goalID, itemID, title string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) || !validID(itemID) {
		return Goal{}, ErrInvalidInput
	}
	title, err := cleanTitle(title)
	if err != nil {
		return Goal{}, err
	}
	if err := s.repo.renameItem(ctx, owner, goalID, itemID, title); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) SetItemCompleted(ctx context.Context, owner identity.OwnerID, goalID, itemID string, completed bool) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) || !validID(itemID) {
		return Goal{}, ErrInvalidInput
	}
	if err := s.repo.setItemCompleted(ctx, owner, goalID, itemID, completed); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func (s *Service) RemoveItem(ctx context.Context, owner identity.OwnerID, goalID, itemID string) (Goal, error) {
	if !owner.Valid() {
		return Goal{}, ErrUnauthorized
	}
	if !validID(goalID) || !validID(itemID) {
		return Goal{}, ErrInvalidInput
	}
	if err := s.repo.removeItem(ctx, owner, goalID, itemID); err != nil {
		return Goal{}, err
	}
	return s.GetGoal(ctx, owner, goalID)
}

func cleanTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 160 || strings.ContainsRune(title, 0) {
		return "", ErrInvalidInput
	}
	return title, nil
}

func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func newUUID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate Goal ID: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}
