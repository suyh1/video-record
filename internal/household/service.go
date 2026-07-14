package household

import (
	"context"
	"errors"
	"strings"
	"time"

	"video-record/internal/auth"
)

var (
	ErrForbidden       = errors.New("household action forbidden")
	ErrInvalidMember   = errors.New("invalid household member")
	ErrMemberNotFound  = errors.New("household member not found")
	ErrRecordNotFound  = errors.New("household record not found")
	ErrVersionConflict = errors.New("household record version conflict")
)

type Member struct {
	ID        string
	Username  string
	Role      auth.Role
	Active    bool
	CreatedAt time.Time
}

type SharingInput struct {
	ShareRating     bool
	ShareReview     bool
	SharedReview    string
	ExpectedVersion int
}

type Sharing struct {
	OwnerID      string
	MediaID      string
	ShareRating  bool
	ShareReview  bool
	SharedReview *string
	Version      int
}

type VisibleRecord struct {
	OwnerID      string
	MediaID      string
	Rating       *int
	PrivateNote  *string
	SharedReview *string
}

type SharedEvent struct {
	ID           string    `json:"id"`
	MediaID      string    `json:"mediaId"`
	Title        string    `json:"title"`
	WatchedAt    time.Time `json:"watchedAt"`
	Participants []string  `json:"participants"`
}

type recordPrivacy struct {
	OwnerID      string
	MediaID      string
	Rating       *int
	PrivateNote  *string
	ShareRating  bool
	ShareReview  bool
	SharedReview *string
	Version      int
}

type Repository interface {
	FindMember(context.Context, string) (Member, error)
	Members(context.Context) ([]Member, error)
	CreateMember(context.Context, Member, string, string) error
	ResetPassword(context.Context, string, string, string, time.Time) error
	DeactivateMember(context.Context, string, string, time.Time) error
	RecordPrivacy(context.Context, string, string) (recordPrivacy, error)
	UpdateSharing(context.Context, string, string, SharingInput) (recordPrivacy, error)
	SharedEvents(context.Context, string) ([]SharedEvent, error)
}

type Service struct {
	repository Repository
	policy     Policy
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (service *Service) Members(ctx context.Context, actorID string) ([]Member, error) {
	if err := service.requireAdmin(ctx, actorID); err != nil {
		return nil, err
	}
	return service.repository.Members(ctx)
}

func (service *Service) Participants(ctx context.Context, viewerID string) ([]Member, error) {
	viewer, err := service.repository.FindMember(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	if !viewer.Active {
		return nil, ErrForbidden
	}
	members, err := service.repository.Members(ctx)
	if err != nil {
		return nil, err
	}
	participants := make([]Member, 0, len(members))
	for _, member := range members {
		if member.Active && member.ID != viewerID {
			participants = append(participants, member)
		}
	}
	return participants, nil
}

func (service *Service) CreateMember(ctx context.Context, actorID, username, password string) (Member, error) {
	if err := service.requireAdmin(ctx, actorID); err != nil {
		return Member{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 12 {
		return Member{}, ErrInvalidMember
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return Member{}, err
	}
	member := Member{
		ID: authID(), Username: username, Role: auth.RoleMember,
		Active: true, CreatedAt: service.now().UTC(),
	}
	if err := service.repository.CreateMember(ctx, member, passwordHash, actorID); err != nil {
		return Member{}, err
	}
	return member, nil
}

func (service *Service) ResetPassword(ctx context.Context, actorID, targetID, password string) error {
	if err := service.requireManagedMember(ctx, actorID, targetID); err != nil {
		return err
	}
	if len(password) < 12 {
		return ErrInvalidMember
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return service.repository.ResetPassword(ctx, actorID, targetID, passwordHash, service.now().UTC())
}

func (service *Service) DeactivateMember(ctx context.Context, actorID, targetID string) error {
	if err := service.requireManagedMember(ctx, actorID, targetID); err != nil {
		return err
	}
	return service.repository.DeactivateMember(ctx, actorID, targetID, service.now().UTC())
}

func (service *Service) UpdateSharing(
	ctx context.Context,
	actorID, ownerID, mediaID string,
	input SharingInput,
) (Sharing, error) {
	if !service.policy.CanMutatePersonalRecord(actorID, ownerID) {
		return Sharing{}, ErrForbidden
	}
	input.SharedReview = strings.TrimSpace(input.SharedReview)
	if mediaID == "" || input.ExpectedVersion < 1 || len(input.SharedReview) > 500 || (input.ShareReview && input.SharedReview == "") {
		return Sharing{}, ErrInvalidMember
	}
	if !input.ShareReview {
		input.SharedReview = ""
	}
	record, err := service.repository.UpdateSharing(ctx, ownerID, mediaID, input)
	if err != nil {
		return Sharing{}, err
	}
	return newSharing(record), nil
}

func (service *Service) Sharing(ctx context.Context, actorID, ownerID, mediaID string) (Sharing, error) {
	if !service.policy.CanMutatePersonalRecord(actorID, ownerID) {
		return Sharing{}, ErrForbidden
	}
	record, err := service.repository.RecordPrivacy(ctx, ownerID, mediaID)
	if err != nil {
		return Sharing{}, err
	}
	return newSharing(record), nil
}

func (service *Service) VisibleRecord(ctx context.Context, viewerID, ownerID, mediaID string) (VisibleRecord, error) {
	if _, err := service.repository.FindMember(ctx, viewerID); err != nil {
		return VisibleRecord{}, err
	}
	if _, err := service.repository.FindMember(ctx, ownerID); err != nil {
		return VisibleRecord{}, err
	}
	record, err := service.repository.RecordPrivacy(ctx, ownerID, mediaID)
	if err != nil {
		return VisibleRecord{}, err
	}
	visible := VisibleRecord{OwnerID: ownerID, MediaID: mediaID}
	if viewerID == ownerID || record.ShareRating {
		visible.Rating = record.Rating
	}
	if service.policy.CanReadPrivateNote(viewerID, ownerID) {
		visible.PrivateNote = record.PrivateNote
	}
	if viewerID == ownerID || record.ShareReview {
		visible.SharedReview = record.SharedReview
	}
	return visible, nil
}

func (service *Service) SharedEvents(ctx context.Context, viewerID string) ([]SharedEvent, error) {
	if _, err := service.repository.FindMember(ctx, viewerID); err != nil {
		return nil, err
	}
	return service.repository.SharedEvents(ctx, viewerID)
}

func (service *Service) requireAdmin(ctx context.Context, actorID string) error {
	actor, err := service.repository.FindMember(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Active || !service.policy.CanManageMembers(actor.Role) {
		return ErrForbidden
	}
	return nil
}

func (service *Service) requireManagedMember(ctx context.Context, actorID, targetID string) error {
	if err := service.requireAdmin(ctx, actorID); err != nil {
		return err
	}
	target, err := service.repository.FindMember(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Role != auth.RoleMember || actorID == targetID {
		return ErrForbidden
	}
	return nil
}

func newSharing(record recordPrivacy) Sharing {
	return Sharing{
		OwnerID: record.OwnerID, MediaID: record.MediaID,
		ShareRating: record.ShareRating, ShareReview: record.ShareReview,
		SharedReview: record.SharedReview, Version: record.Version,
	}
}
