package main

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestMovieJSONMarshalling(t *testing.T) {
	// Create a sample movie with fields that would be marshalled
	movie := Movie{
		ID:        1,
		Title:     "Test Movie",
		Duration:  120,
		Rating:    "8.5",
		Formats:   []string{"2D", "3D", "IMAX"},
		Languages: []string{"English", "Hindi"},
		Description: sql.NullString{
			String: "This is a test movie description",
			Valid:  true,
		},
	}

	// Test marshalling
	movieJSON, err := json.Marshal(movie)
	if err != nil {
		t.Fatalf("Failed to marshal movie to JSON: %v", err)
	}

	// Test unmarshalling
	var unmarshalledMovie Movie
	err = json.Unmarshal(movieJSON, &unmarshalledMovie)
	if err != nil {
		t.Fatalf("Failed to unmarshal movie from JSON: %v", err)
	}

	// Check if data matches after marshalling and unmarshalling
	if movie.ID != unmarshalledMovie.ID {
		t.Errorf("ID does not match after unmarshalling: got %d, want %d",
			unmarshalledMovie.ID, movie.ID)
	}
	if movie.Title != unmarshalledMovie.Title {
		t.Errorf("Title does not match after unmarshalling: got %s, want %s",
			unmarshalledMovie.Title, movie.Title)
	}
	if !reflect.DeepEqual(movie.Formats, unmarshalledMovie.Formats) {
		t.Errorf("Formats do not match after unmarshalling: got %v, want %v",
			unmarshalledMovie.Formats, movie.Formats)
	}
	if !reflect.DeepEqual(movie.Languages, unmarshalledMovie.Languages) {
		t.Errorf("Languages do not match after unmarshalling: got %v, want %v",
			unmarshalledMovie.Languages, movie.Languages)
	}
}

func TestBookingJSONMarshalling(t *testing.T) {
	// Create sample seats that would be marshalled
	seats := []int{1, 5, 10, 15}

	// Test marshalling seats to JSON
	seatsJSON, err := json.Marshal(seats)
	if err != nil {
		t.Fatalf("Failed to marshal seats to JSON: %v", err)
	}

	// Test unmarshalling seats from JSON
	var unmarshalledSeats []int
	err = json.Unmarshal(seatsJSON, &unmarshalledSeats)
	if err != nil {
		t.Fatalf("Failed to unmarshal seats from JSON: %v", err)
	}

	// Check if the seats match after marshalling and unmarshalling
	if !reflect.DeepEqual(seats, unmarshalledSeats) {
		t.Errorf("Seats do not match after unmarshalling: got %v, want %v",
			unmarshalledSeats, seats)
	}

	// Create a sample booking
	booking := Booking{
		ID:         1,
		ShowtimeID: 2,
		Customer:   "John Doe",
		Email:      "john@example.com",
		Phone:      "1234567890",
		Seats:      seats,
		TotalPrice: 1000.0,
		Status:     "confirmed",
		CreatedAt:  time.Now(),
		Showtime: Showtime{
			ID:       2,
			MovieID:  3,
			DateTime: time.Now(),
			Hall:     "Hall A",
			Format:   "2D",
		},
	}

	// Test marshalling booking to JSON
	bookingJSON, err := json.Marshal(booking)
	if err != nil {
		t.Fatalf("Failed to marshal booking to JSON: %v", err)
	}

	// Test unmarshalling booking from JSON
	var unmarshalledBooking Booking
	err = json.Unmarshal(bookingJSON, &unmarshalledBooking)
	if err != nil {
		t.Fatalf("Failed to unmarshal booking from JSON: %v", err)
	}

	// Check if the booking data matches after marshalling and unmarshalling
	if booking.ID != unmarshalledBooking.ID {
		t.Errorf("ID does not match after unmarshalling: got %d, want %d",
			unmarshalledBooking.ID, booking.ID)
	}
	if booking.Customer != unmarshalledBooking.Customer {
		t.Errorf("Customer does not match after unmarshalling: got %s, want %s",
			unmarshalledBooking.Customer, booking.Customer)
	}
	if !reflect.DeepEqual(booking.Seats, unmarshalledBooking.Seats) {
		t.Errorf("Seats do not match after unmarshalling: got %v, want %v",
			unmarshalledBooking.Seats, booking.Seats)
	}
}

func TestMarshalSeatLabels(t *testing.T) {
	// Test case for generating and encoding seat labels
	seatNumbers := []int{1, 12, 24, 48, 49, 60}

	// Generate seat labels
	labels := generateSeatLabels(seatNumbers)

	// Expected labels based on our seat mapping logic
	expectedLabels := []string{"A1", "A12", "B12", "D12", "E1", "E12"}

	// Check if the generated labels match expected values
	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("Seat labels don't match expected values: got %v, want %v",
			labels, expectedLabels)
	}

	// Create a structure with the labels
	seatData := struct {
		Labels []string `json:"labels"`
	}{
		Labels: labels,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(seatData)
	if err != nil {
		t.Fatalf("Failed to marshal seat labels to JSON: %v", err)
	}

	// Unmarshal from JSON
	var unmarshalledData struct {
		Labels []string `json:"labels"`
	}
	err = json.Unmarshal(jsonData, &unmarshalledData)
	if err != nil {
		t.Fatalf("Failed to unmarshal seat labels from JSON: %v", err)
	}

	// Check if labels match after marshalling and unmarshalling
	if !reflect.DeepEqual(seatData.Labels, unmarshalledData.Labels) {
		t.Errorf("Seat labels don't match after JSON roundtrip: got %v, want %v",
			unmarshalledData.Labels, seatData.Labels)
	}
}
