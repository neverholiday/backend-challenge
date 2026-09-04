package mongodb

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// UserRepository is a MongoDB-backed implementation of domain.UserRepository.
type UserRepository struct {
	collection *mongo.Collection
}

var _ domain.UserRepository = (*UserRepository)(nil)

// NewUserRepository builds a UserRepository against the "users" collection of db.
func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{collection: db.Collection("users")}
}

// EnsureIndexes creates the indexes UserRepository depends on. Call it once at
// startup — it is not invoked from NewUserRepository so construction stays
// non-blocking and testable without a live round-trip.
func (r *UserRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

// CreateUser inserts user, returning domain.ErrEmailAlreadyExists if its
// email violates the unique index.
func (r *UserRepository) CreateUser(ctx context.Context, user domain.User) error {
	_, err := r.collection.InsertOne(ctx, fromDomain(user))
	if mongo.IsDuplicateKeyError(err) {
		return domain.ErrEmailAlreadyExists
	}
	return err
}

// CountUsers returns the total number of users in the collection.
func (r *UserRepository) CountUsers(ctx context.Context) (uint, error) {
	count, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, nil
	}
	return uint(count), nil
}

// GetUserByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: id}})
}

// GetUserByEmail returns the user with the given email, or domain.ErrUserNotFound.
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.D) (*domain.User, error) {
	var doc userDocument
	err := r.collection.FindOne(ctx, filter).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	user := doc.toDomain()
	return &user, nil
}

// ListUsers returns every user in the collection.
func (r *UserRepository) ListUsers(ctx context.Context) ([]domain.User, error) {
	cursor, err := r.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var docs []userDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	users := make([]domain.User, len(docs))
	for i, doc := range docs {
		users[i] = doc.toDomain()
	}
	return users, nil
}

// UpdateUser applies the non-nil fields of param to the user with the given id
// and returns the resulting document, or domain.ErrUserNotFound if no such
// user exists, or domain.ErrEmailAlreadyExists if the new email violates the
// unique index.
//
// FindOneAndUpdate rather than an update followed by a read: the write and the
// read of its result are one operation, so a concurrent update cannot land in
// between and make this return state the caller never wrote.
func (r *UserRepository) UpdateUser(
	ctx context.Context,
	id string,
	param domain.UserUpdateParam,
) (*domain.User, error) {
	set := bson.D{}
	if param.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *param.Name})
	}
	if param.Email != nil {
		set = append(set, bson.E{Key: "email", Value: *param.Email})
	}
	if len(set) == 0 {
		return r.GetUserByID(ctx, id)
	}

	var doc userDocument
	err := r.collection.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: set}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&doc)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrUserNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return nil, domain.ErrEmailAlreadyExists
	}
	if err != nil {
		return nil, err
	}

	user := doc.toDomain()
	return &user, nil
}

// DeleteUser removes the user with the given id, or returns domain.ErrUserNotFound.
func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}
