package database

import (
	"crypto/rand"
)

func (s *sqliteDB) GetUser(userID int64) (*User, error) {
	user := &User{}
	err := s.db.QueryRow("SELECT id, public_id, first_name, username, created_at FROM users WHERE id = ?", userID).Scan(
		&user.ID,
		&user.PublicID,
		&user.FirstName,
		&user.Username,
		&user.CreatedAt,
	)
	if err != nil {
		return user, err
	}

	return user, nil
}

func (s *sqliteDB) SaveUser(user User) error {
	if user.PublicID == "" {
		publicID, err := generatePublicID()
		if err != nil {
			return err
		}
		user.PublicID = publicID
	}

	_, err := s.db.Exec(`
		INSERT INTO users (id, public_id, first_name, username) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			first_name = excluded.first_name,
			username = excluded.username,
			public_id = excluded.public_id,
			updated_at = CURRENT_TIMESTAMP
	`, user.ID, user.PublicID, user.FirstName, user.Username)
	return err
}

func generatePublicID() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	num := uint32(bytes[0])<<16 | uint32(bytes[1])<<8 | uint32(bytes[2])
	return base36Encode(num), nil
}

func base36Encode(num uint32) string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	var result []byte
	for num > 0 {
		result = append([]byte{charset[num%36]}, result...)
		num /= 36
	}

	for len(result) < 4 {
		result = append([]byte{'0'}, result...)
	}

	if len(result) > 4 {
		result = result[:4]
	}
	return string(result)
}
