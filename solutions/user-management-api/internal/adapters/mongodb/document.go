package mongodb

import (
	"time"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// userDocument is the BSON persistence shape for domain.User. It exists so the
// domain package never needs to know about MongoDB's field naming or types.
type userDocument struct {
	ID           string    `bson:"_id"`
	Name         string    `bson:"name"`
	Email        string    `bson:"email"`
	PasswordHash string    `bson:"password_hash"`
	CreatedAt    time.Time `bson:"created_at"`
}

func fromDomain(user domain.User) userDocument {
	return userDocument{
		ID:           user.ID,
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		CreatedAt:    user.CreatedAt,
	}
}

func (d userDocument) toDomain() domain.User {
	return domain.User{
		ID:           d.ID,
		Name:         d.Name,
		Email:        d.Email,
		PasswordHash: d.PasswordHash,
		CreatedAt:    d.CreatedAt,
	}
}
