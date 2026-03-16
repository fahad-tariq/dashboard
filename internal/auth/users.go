package auth

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a registered dashboard user.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    string
}

// ValidatePassword checks that a password meets minimum requirements.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	return nil
}

// CreateUser hashes the password with bcrypt and inserts a new user row.
// The first user created is automatically assigned the admin role.
func CreateUser(db *sql.DB, email, password string) (int64, error) {
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hashing password: %w", err)
	}
	return CreateUserWithHash(db, email, string(hash))
}

// CreateUserWithHash inserts a new user row with a pre-computed password hash.
// The first user created is automatically assigned the admin role.
func CreateUserWithHash(db *sql.DB, email, hash string) (int64, error) {
	role := "user"
	count, err := UserCount(db)
	if err != nil {
		return 0, fmt.Errorf("checking user count: %w", err)
	}
	if count == 0 {
		role = "admin"
	}

	result, err := db.Exec(
		"INSERT INTO users (email, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
		email, hash, role, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting user: %w", err)
	}
	return result.LastInsertId()
}

// FindByEmail returns the user with the given email, or nil if not found.
func FindByEmail(db *sql.DB, email string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		"SELECT id, email, password_hash, role, created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return u, nil
}

// FindByID returns the user with the given ID, or nil if not found.
func FindByID(db *sql.DB, id int64) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		"SELECT id, email, password_hash, role, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}
	return u, nil
}

// UserCount returns the number of users in the database.
func UserCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// AdminCount returns the number of users with the admin role.
func AdminCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	return count, err
}

// AllUsers returns all users ordered by ID.
// The returned User structs have PasswordHash left empty because callers
// (admin list, startup provisioning) never need it.
func AllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, email, role, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUserEmail changes a user's email address.
func UpdateUserEmail(db *sql.DB, id int64, newEmail string) error {
	_, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", newEmail, id)
	if err != nil {
		return fmt.Errorf("updating email: %w", err)
	}
	return nil
}

// UpdateUserPassword hashes the new password and updates the user's row.
func UpdateUserPassword(db *sql.DB, id int64, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	_, err = db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hash), id)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	return nil
}

// UpdateUserRole sets the user's role. Role must be "admin" or "user".
func UpdateUserRole(db *sql.DB, id int64, role string) error {
	if role != "admin" && role != "user" {
		return fmt.Errorf("invalid role %q: must be 'admin' or 'user'", role)
	}
	_, err := db.Exec("UPDATE users SET role = ? WHERE id = ?", role, id)
	if err != nil {
		return fmt.Errorf("updating role: %w", err)
	}
	return nil
}

// DeleteUser removes a user and all associated data in a transaction.
// Cascades: deletes tracker_items and sessions for the user.
func DeleteUser(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tracker_items WHERE user_id = ?", id); err != nil {
		return fmt.Errorf("deleting tracker items: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return fmt.Errorf("deleting sessions: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM users WHERE id = ?", id); err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}

	return tx.Commit()
}

// InvalidateSessions removes all sessions for a specific user.
func InvalidateSessions(db *sql.DB, userID int64) error {
	_, err := db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	if err != nil {
		return fmt.Errorf("invalidating sessions: %w", err)
	}
	return nil
}
