package database

import "time"

type Subordinate struct {
	ID         int    `json:"id"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
}

type Leave struct {
	ID            int       `json:"id"`
	SubordinateID int       `json:"subordinate_id"`
	LeaveTime     time.Time `json:"leave_time"`
	CreatedAt     time.Time `json:"created_at"`
}

type UnplannedActivity struct {
	ID            int       `json:"id"`
	SubordinateID int       `json:"subordinate_id"`
	ActivityTime  time.Time `json:"activity_time"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}
