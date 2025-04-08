package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Movie represents a movie with all its details
type Movie struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Description sql.NullString `json:"description"`
	Duration    int            `json:"duration"`
	Genre       string         `json:"genre"`
	Director    string         `json:"director"`
	Cast        sql.NullString `json:"cast"`
	Rating      string         `json:"rating"`
	VoteCount   int            `json:"vote_count"`
	PosterURL   sql.NullString `json:"poster_url"`
	BackdropURL sql.NullString `json:"backdrop_url"`
	ReleaseDate sql.NullTime   `json:"release_date"`
	Languages   []string       `json:"languages"`
	Formats     []string       `json:"formats"`
	IsUpcoming  bool           `json:"is_upcoming"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Showtime represents a movie showtime
type Showtime struct {
	ID        int       `json:"id"`
	MovieID   int       `json:"movie_id"`
	Movie     Movie     `json:"movie"`
	DateTime  time.Time `json:"date_time"`
	Time      time.Time `json:"time"`    // For template display
	Theater   string    `json:"theater"` // Venue name
	Screen    string    `json:"screen"`  // Screen number/name
	Hall      string    `json:"hall"`    // Original field
	Price     float64   `json:"price"`
	Available int       `json:"available"`
	Seats     []int     `json:"seats,omitempty"`
	Format    string    `json:"format"`
	Formats   []string  `json:"formats"` // For multiple formats
}

// Booking represents a ticket booking
type Booking struct {
	ID         int       `json:"id"`
	ShowtimeID int       `json:"showtime_id"`
	Showtime   Showtime  `json:"showtime"`
	Customer   string    `json:"customer"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	Seats      []int     `json:"seats"`
	TotalPrice float64   `json:"total_price"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// BookingRequest represents the incoming booking request
type BookingRequest struct {
	ShowtimeID    int    `json:"showtime_id"`
	Customer      string `json:"customer"`
	SelectedSeats []int  `json:"selected_seats"`
}

var (
	db  *sql.DB
	tpl *template.Template
	// Mutex for each showtime's seat operations
	showtimeMutexes = make(map[int]*sync.Mutex)
	// Global mutex to protect the showtime mutexes map
	mutexMapLock sync.RWMutex
)

// getShowtimeMutex returns a mutex for a specific showtime
func getShowtimeMutex(showtimeID int) *sync.Mutex {
	mutexMapLock.RLock()
	mutex, exists := showtimeMutexes[showtimeID]
	mutexMapLock.RUnlock()

	if !exists {
		mutexMapLock.Lock()
		mutex = &sync.Mutex{}
		showtimeMutexes[showtimeID] = mutex
		mutexMapLock.Unlock()
	}

	return mutex
}

func init() {
	var err error
	funcMap := template.FuncMap{
		"add":          add,
		"isSeatBooked": isSeatBooked,
	}
	tpl = template.New("").Funcs(funcMap)
	tpl, err = tpl.ParseGlob("templates/*.gohtml")
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// Open database connection
	var err error
	db, err = sql.Open("mysql", "root:1234@tcp(127.0.0.1:3306)/cinema_db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to MySQL database!")

	// Create necessary tables if they don't exist
	createTables()

	// User routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/movies", handleMovies)
	http.HandleFunc("/movie/", handleMovieRouter)
	http.HandleFunc("/book/", handleBooking)
	http.HandleFunc("/bookings", handleMyBookings)

	// New booking flow routes
	http.HandleFunc("/booking/seats", handleSeatSelection)
	http.HandleFunc("/booking/payment", handlePayment)
	http.HandleFunc("/booking/confirm", handleBookingConfirm)

	// Admin routes
	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/movie/add", handleAddMovie)
	http.HandleFunc("/admin/showtime/add", handleAddShowtime)

	// Start server
	fmt.Println("Server is running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func createTables() {
	// Create movies table
	movieQuery := `CREATE TABLE IF NOT EXISTS movies (
		id INT AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		description TEXT,
		duration INT NOT NULL,
		genre VARCHAR(100),
		director VARCHAR(100),
		cast TEXT,
		rating VARCHAR(10) DEFAULT '',
		vote_count INT DEFAULT 0,
		poster_url TEXT,
		backdrop_url TEXT,
		release_date DATE,
		languages JSON,
		formats JSON,
		is_upcoming BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	// Create showtimes table
	showtimeQuery := `CREATE TABLE IF NOT EXISTS showtimes (
		id INT AUTO_INCREMENT PRIMARY KEY,
		movie_id INT NOT NULL,
		date_time DATETIME NOT NULL,
		hall VARCHAR(50) NOT NULL,
		price DECIMAL(10,2) NOT NULL,
		available INT NOT NULL,
		seats JSON,
		format VARCHAR(20),
		FOREIGN KEY (movie_id) REFERENCES movies(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	// Create bookings table
	bookingQuery := `CREATE TABLE IF NOT EXISTS bookings (
		id INT AUTO_INCREMENT PRIMARY KEY,
		showtime_id INT NOT NULL,
		customer VARCHAR(100) NOT NULL,
		email VARCHAR(100) NOT NULL,
		phone VARCHAR(20) NOT NULL,
		seats JSON NOT NULL,
		total_price DECIMAL(10,2) NOT NULL,
		status VARCHAR(20) DEFAULT 'confirmed',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (showtime_id) REFERENCES showtimes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	// Create transaction logs table
	transactionQuery := `CREATE TABLE IF NOT EXISTS transaction_logs (
		id INT AUTO_INCREMENT PRIMARY KEY,
		booking_id INT NOT NULL,
		payment_method VARCHAR(50) NOT NULL,
		amount DECIMAL(10,2) NOT NULL,
		status VARCHAR(20) NOT NULL,
		transaction_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	_, err := db.Exec(movieQuery)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(showtimeQuery)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(bookingQuery)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(transactionQuery)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Database tables created successfully")
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	// Get Now Showing movies
	nowShowing, err := getMovies(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get Upcoming movies
	upcoming, err := getMovies(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Movies         []Movie
		UpcomingMovies []Movie
	}{
		Movies:         nowShowing,
		UpcomingMovies: upcoming,
	}

	err = tpl.ExecuteTemplate(w, "index.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getMovies(upcoming bool) ([]Movie, error) {
	query := `SELECT id, title, description, duration, genre, director, cast, 
			  rating, vote_count, poster_url, backdrop_url, release_date, 
			  languages, formats, is_upcoming 
			  FROM movies WHERE is_upcoming = ? ORDER BY release_date`

	rows, err := db.Query(query, upcoming)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []Movie
	for rows.Next() {
		var m Movie
		var languagesJSON, formatsJSON sql.NullString
		var releaseDateStr sql.NullString

		err := rows.Scan(&m.ID, &m.Title, &m.Description, &m.Duration, &m.Genre,
			&m.Director, &m.Cast, &m.Rating, &m.VoteCount, &m.PosterURL,
			&m.BackdropURL, &releaseDateStr, &languagesJSON, &formatsJSON,
			&m.IsUpcoming)
		if err != nil {
			return nil, err
		}

		// Parse the release date string into a time.Time
		if releaseDateStr.Valid {
			releaseDate, err := time.Parse("2006-01-02", releaseDateStr.String)
			if err == nil {
				m.ReleaseDate.Valid = true
				m.ReleaseDate.Time = releaseDate
			}
		}

		if languagesJSON.Valid {
			json.Unmarshal([]byte(languagesJSON.String), &m.Languages)
		}
		if formatsJSON.Valid {
			json.Unmarshal([]byte(formatsJSON.String), &m.Formats)
		}

		movies = append(movies, m)
	}

	return movies, nil
}

func handleMovies(w http.ResponseWriter, r *http.Request) {
	// Get all movies
	movies, err := getMovies(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Movies []Movie
	}{
		Movies: movies,
	}

	err = tpl.ExecuteTemplate(w, "movies.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleMovieRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check if the URL matches /movie/ID/book pattern
	if strings.Contains(path, "/book") {
		handleBooking(w, r)
		return
	}

	// Otherwise handle as a movie details request
	handleMovie(w, r)
}

func handleMovie(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/movie/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	movieID, err := strconv.Atoi(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	movie, err := getMovie(movieID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	data := struct {
		Movie Movie
	}{
		Movie: movie,
	}

	err = tpl.ExecuteTemplate(w, "movie.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getMovie(id int) (Movie, error) {
	var m Movie
	var languagesJSON, formatsJSON sql.NullString
	var releaseDateStr sql.NullString

	query := `SELECT id, title, description, duration, genre, director, cast, 
			  rating, vote_count, poster_url, backdrop_url, release_date, 
			  languages, formats, is_upcoming 
			  FROM movies WHERE id = ?`

	err := db.QueryRow(query, id).Scan(
		&m.ID, &m.Title, &m.Description, &m.Duration, &m.Genre,
		&m.Director, &m.Cast, &m.Rating, &m.VoteCount, &m.PosterURL,
		&m.BackdropURL, &releaseDateStr, &languagesJSON, &formatsJSON,
		&m.IsUpcoming)

	if err != nil {
		return m, err
	}

	// Parse the release date string into a time.Time
	if releaseDateStr.Valid {
		releaseDate, err := time.Parse("2006-01-02", releaseDateStr.String)
		if err == nil {
			m.ReleaseDate.Valid = true
			m.ReleaseDate.Time = releaseDate
		}
	}

	if languagesJSON.Valid {
		json.Unmarshal([]byte(languagesJSON.String), &m.Languages)
	}
	if formatsJSON.Valid {
		json.Unmarshal([]byte(formatsJSON.String), &m.Formats)
	}

	return m, nil
}

// Showtime handlers
func showAllShowtimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
		SELECT s.id, s.movie_id, DATE_FORMAT(s.date_time, '%Y-%m-%d %H:%i:%s'), s.hall, s.price, s.available,
		       m.id, m.title, m.description, m.duration, m.rating
		FROM showtimes s
		JOIN movies m ON s.movie_id = m.id
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	showtimes := []Showtime{}
	for rows.Next() {
		var s Showtime
		var m Movie
		var dateTimeStr string
		err := rows.Scan(&s.ID, &s.MovieID, &dateTimeStr, &s.Hall, &s.Price, &s.Available,
			&m.ID, &m.Title, &m.Description, &m.Duration, &m.Rating)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Parse the datetime string
		s.DateTime, err = time.Parse("2006-01-02 15:04:05", dateTimeStr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.Movie = m
		showtimes = append(showtimes, s)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tpl.ExecuteTemplate(w, "showtimes.gohtml", showtimes)
}

func createShowtimeForm(w http.ResponseWriter, r *http.Request) {
	// Get all movies for the dropdown
	rows, err := db.Query("SELECT id, title FROM movies")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	movies := []Movie{}
	for rows.Next() {
		var m Movie
		err := rows.Scan(&m.ID, &m.Title)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		movies = append(movies, m)
	}

	tpl.ExecuteTemplate(w, "create_showtime.gohtml", movies)
}

func insertShowtime(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	movieIDStr := r.FormValue("movie_id")
	dateTimeStr := r.FormValue("date_time")
	hall := r.FormValue("hall")
	priceStr := r.FormValue("price")
	availableStr := r.FormValue("available")

	movieID, err := strconv.Atoi(movieIDStr)
	if err != nil {
		http.Error(w, "Invalid movie ID", http.StatusBadRequest)
		return
	}

	// Parse the datetime string from the form
	dateTime, err := time.Parse("2006-01-02T15:04", dateTimeStr)
	if err != nil {
		http.Error(w, "Invalid date/time format", http.StatusBadRequest)
		return
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		http.Error(w, "Invalid price", http.StatusBadRequest)
		return
	}

	available, err := strconv.Atoi(availableStr)
	if err != nil {
		http.Error(w, "Invalid available seats", http.StatusBadRequest)
		return
	}

	// Format the datetime for MySQL
	formattedDateTime := dateTime.Format("2006-01-02 15:04:05")

	_, err = db.Exec("INSERT INTO showtimes (movie_id, date_time, hall, price, available) VALUES (?, ?, ?, ?, ?)",
		movieID, formattedDateTime, hall, price, available)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/showtimes", http.StatusSeeOther)
}

func editShowtimeForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/showtime/edit/"):]

	// Get showtime details
	row := db.QueryRow(`
		SELECT s.id, s.movie_id, DATE_FORMAT(s.date_time, '%Y-%m-%d %H:%i:%s'), s.hall, s.price, s.available,
		       m.id, m.title, m.description, m.duration, m.rating
		FROM showtimes s
		JOIN movies m ON s.movie_id = m.id
		WHERE s.id = ?
	`, id)

	var s Showtime
	var m Movie
	var dateTimeStr string
	err := row.Scan(&s.ID, &s.MovieID, &dateTimeStr, &s.Hall, &s.Price, &s.Available,
		&m.ID, &m.Title, &m.Description, &m.Duration, &m.Rating)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Parse the datetime string
	s.DateTime, err = time.Parse("2006-01-02 15:04:05", dateTimeStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Movie = m

	// Get all movies for the dropdown
	rows, err := db.Query("SELECT id, title FROM movies")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	movies := []Movie{}
	for rows.Next() {
		var movie Movie
		err := rows.Scan(&movie.ID, &movie.Title)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		movies = append(movies, movie)
	}

	data := struct {
		Showtime Showtime
		Movies   []Movie
	}{
		Showtime: s,
		Movies:   movies,
	}

	tpl.ExecuteTemplate(w, "edit_showtime.gohtml", data)
}

func updateShowtime(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	movieIDStr := r.FormValue("movie_id")
	dateTimeStr := r.FormValue("date_time")
	hall := r.FormValue("hall")
	priceStr := r.FormValue("price")
	availableStr := r.FormValue("available")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	movieID, err := strconv.Atoi(movieIDStr)
	if err != nil {
		http.Error(w, "Invalid movie ID", http.StatusBadRequest)
		return
	}

	dateTime, err := time.Parse("2006-01-02T15:04", dateTimeStr)
	if err != nil {
		http.Error(w, "Invalid date/time", http.StatusBadRequest)
		return
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		http.Error(w, "Invalid price", http.StatusBadRequest)
		return
	}

	available, err := strconv.Atoi(availableStr)
	if err != nil {
		http.Error(w, "Invalid available seats", http.StatusBadRequest)
		return
	}

	// Format the datetime for MySQL
	formattedDateTime := dateTime.Format("2006-01-02 15:04:05")

	_, err = db.Exec(`
		UPDATE showtimes 
		SET movie_id = ?, date_time = ?, hall = ?, price = ?, available = ?
		WHERE id = ?
	`, movieID, formattedDateTime, hall, price, available, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/showtimes", http.StatusSeeOther)
}

func deleteShowtime(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/showtime/delete/"):]

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Delete showtime
	_, err = tx.Exec("DELETE FROM showtimes WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/showtimes", http.StatusSeeOther)
}

func handleShowtime(w http.ResponseWriter, r *http.Request) {
	// Extract showtime ID from URL
	path := r.URL.Path[len("/api/showtime/"):]
	if len(path) == 0 {
		http.Error(w, "Missing showtime ID", http.StatusBadRequest)
		return
	}

	// Check if this is a seats request
	if len(path) > 6 && path[len(path)-6:] == "/seats" {
		showtimeID, err := strconv.Atoi(path[:len(path)-6])
		if err != nil {
			http.Error(w, "Invalid showtime ID", http.StatusBadRequest)
			return
		}
		getShowtimeSeats(w, r, showtimeID)
		return
	}

	http.Error(w, "Invalid endpoint", http.StatusNotFound)
}

func getShowtimeSeats(w http.ResponseWriter, r *http.Request, showtimeID int) {
	// Get mutex for this showtime
	mutex := getShowtimeMutex(showtimeID)
	mutex.Lock()
	defer mutex.Unlock()

	// Get booked seats from database
	var seatsJSON sql.NullString
	err := db.QueryRow("SELECT seats FROM showtimes WHERE id = ?", showtimeID).Scan(&seatsJSON)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var bookedSeats []int
	if seatsJSON.Valid {
		err = json.Unmarshal([]byte(seatsJSON.String), &bookedSeats)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return booked seats as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookedSeats)
}

// Booking handlers
func showAllBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
		SELECT b.id, b.showtime_id, b.customer, b.seats, b.total_price,
		       s.id, s.movie_id, DATE_FORMAT(s.date_time, '%Y-%m-%d %H:%i:%s'), s.hall, s.price, s.available,
		       m.id, m.title, m.description, m.duration, m.rating
		FROM bookings b
		JOIN showtimes s ON b.showtime_id = s.id
		JOIN movies m ON s.movie_id = m.id
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	bookings := []Booking{}
	for rows.Next() {
		var b Booking
		var s Showtime
		var m Movie
		var dateTimeStr string
		err := rows.Scan(&b.ID, &b.ShowtimeID, &b.Customer, &b.Seats, &b.TotalPrice,
			&s.ID, &s.MovieID, &dateTimeStr, &s.Hall, &s.Price, &s.Available,
			&m.ID, &m.Title, &m.Description, &m.Duration, &m.Rating)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Parse the datetime string
		s.DateTime, err = time.Parse("2006-01-02 15:04:05", dateTimeStr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.Movie = m
		b.Showtime = s
		bookings = append(bookings, b)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tpl.ExecuteTemplate(w, "bookings.gohtml", bookings)
}

func createBookingForm(w http.ResponseWriter, r *http.Request) {
	// Get all showtimes for the dropdown
	rows, err := db.Query(`
		SELECT s.id, DATE_FORMAT(s.date_time, '%Y-%m-%d %H:%i:%s'), s.hall, s.price, s.available,
		       m.id, m.title
		FROM showtimes s
		JOIN movies m ON s.movie_id = m.id
		WHERE s.available > 0
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	showtimes := []Showtime{}
	for rows.Next() {
		var s Showtime
		var m Movie
		var dateTimeStr string
		err := rows.Scan(&s.ID, &dateTimeStr, &s.Hall, &s.Price, &s.Available,
			&m.ID, &m.Title)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Parse the datetime string
		s.DateTime, err = time.Parse("2006-01-02 15:04:05", dateTimeStr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.Movie = m
		showtimes = append(showtimes, s)
	}

	data := struct {
		Showtimes []Showtime
	}{
		Showtimes: showtimes,
	}

	tpl.ExecuteTemplate(w, "create_booking.gohtml", data)
}

func insertBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(405), http.StatusMethodNotAllowed)
		return
	}

	showtimeIDStr := r.FormValue("showtime_id")
	customer := r.FormValue("customer")
	selectedSeatsStr := r.FormValue("selected_seats")

	showtimeID, err := strconv.Atoi(showtimeIDStr)
	if err != nil {
		http.Error(w, "Invalid showtime ID", http.StatusBadRequest)
		return
	}

	// Parse selected seats
	var selectedSeats []int
	if selectedSeatsStr != "" {
		for _, seatStr := range strings.Split(selectedSeatsStr, ",") {
			seat, err := strconv.Atoi(seatStr)
			if err != nil {
				http.Error(w, "Invalid seat number", http.StatusBadRequest)
				return
			}
			selectedSeats = append(selectedSeats, seat)
		}
	}

	if len(selectedSeats) == 0 {
		http.Error(w, "No seats selected", http.StatusBadRequest)
		return
	}

	// Get mutex for this showtime
	mutex := getShowtimeMutex(showtimeID)
	mutex.Lock()
	defer mutex.Unlock()

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get showtime details and check seat availability
	var price float64
	var available int
	var seatsJSON sql.NullString
	err = tx.QueryRow("SELECT price, available, seats FROM showtimes WHERE id = ? FOR UPDATE", showtimeID).
		Scan(&price, &available, &seatsJSON)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if enough seats are available
	if len(selectedSeats) > available {
		tx.Rollback()
		http.Error(w, "Not enough seats available", http.StatusBadRequest)
		return
	}

	// Check if selected seats are already booked
	var bookedSeats []int
	if seatsJSON.Valid {
		err = json.Unmarshal([]byte(seatsJSON.String), &bookedSeats)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Check for double booking
	for _, seat := range selectedSeats {
		for _, booked := range bookedSeats {
			if seat == booked {
				tx.Rollback()
				http.Error(w, fmt.Sprintf("Seat %d is already booked", seat), http.StatusConflict)
				return
			}
		}
	}

	// Calculate total price
	totalPrice := price * float64(len(selectedSeats))

	// Convert selected seats to JSON
	selectedSeatsJSON, err := json.Marshal(selectedSeats)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert booking
	_, err = tx.Exec("INSERT INTO bookings (showtime_id, customer, seats, total_price) VALUES (?, ?, ?, ?)",
		showtimeID, customer, selectedSeatsJSON, totalPrice)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update showtime seats and availability
	bookedSeats = append(bookedSeats, selectedSeats...)
	updatedSeatsJSON, err := json.Marshal(bookedSeats)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("UPDATE showtimes SET available = available - ?, seats = ? WHERE id = ?",
		len(selectedSeats), updatedSeatsJSON, showtimeID)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/bookings", http.StatusSeeOther)
}

func deleteBooking(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/booking/delete/"):]

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Get booking details
	var showtimeID int
	var seats []int
	err = tx.QueryRow("SELECT showtime_id, seats FROM bookings WHERE id = ?", id).
		Scan(&showtimeID, &seats)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}

	// Delete booking
	_, err = tx.Exec("DELETE FROM bookings WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}

	// Update available seats
	_, err = tx.Exec("UPDATE showtimes SET available = available + ? WHERE id = ?",
		len(seats), showtimeID)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/bookings", http.StatusSeeOther)
}

type DateOption struct {
	Date string
	Day  string
}

// Add helper functions for seat selection
func isSeatBooked(seat int, bookedSeats []int) bool {
	for _, bookedSeat := range bookedSeats {
		if seat == bookedSeat {
			return true
		}
	}
	return false
}

func add(a, b int) int {
	return a + b
}

// Update handleBooking to redirect to seat selection
func handleBooking(w http.ResponseWriter, r *http.Request) {
	// Extract movie ID from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	movieIDStr := parts[2]
	movieID, err := strconv.Atoi(movieIDStr)
	if err != nil {
		http.Error(w, "Invalid movie ID", http.StatusBadRequest)
		return
	}

	// Get movie details
	movie, err := getMovie(movieID)
	if err != nil {
		http.Error(w, "Failed to get movie: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get dates for next 7 days
	dates := make([]time.Time, 7)
	now := time.Now()
	for i := 0; i < 7; i++ {
		dates[i] = now.AddDate(0, 0, i)
	}

	// Get showtimes for this movie (dummy data for now)
	showtimes := []Showtime{
		{
			ID:        1,
			MovieID:   movieID,
			Movie:     movie,
			DateTime:  time.Now().Add(time.Hour * 2),
			Time:      time.Now().Add(time.Hour * 2),
			Theater:   "PVR Cinemas",
			Screen:    "Screen 1",
			Hall:      "Hall 1",
			Price:     250,
			Available: 120,
			Format:    "2D",
			Formats:   []string{"2D", "Dolby Atmos"},
		},
		{
			ID:        2,
			MovieID:   movieID,
			Movie:     movie,
			DateTime:  time.Now().Add(time.Hour * 5),
			Time:      time.Now().Add(time.Hour * 5),
			Theater:   "PVR Cinemas",
			Screen:    "Screen 2",
			Hall:      "Hall 2",
			Price:     300,
			Available: 100,
			Format:    "3D",
			Formats:   []string{"3D", "IMAX"},
		},
		{
			ID:        3,
			MovieID:   movieID,
			Movie:     movie,
			DateTime:  time.Now().Add(time.Hour*24 + time.Hour*3),
			Time:      time.Now().Add(time.Hour*24 + time.Hour*3),
			Theater:   "INOX Movies",
			Screen:    "Luxury Screen",
			Hall:      "Premium Hall",
			Price:     450,
			Available: 80,
			Format:    "2D",
			Formats:   []string{"2D", "Recliner"},
		},
	}

	// Render template
	data := struct {
		Movie     Movie
		Dates     []time.Time
		Showtimes []Showtime
	}{
		Movie:     movie,
		Dates:     dates,
		Showtimes: showtimes,
	}

	tmpl, err := template.ParseFiles("templates/book.gohtml")
	if err != nil {
		http.Error(w, "Failed to parse template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to execute template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// Add a new handler for seat selection
func handleSeatSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get movie_id from query string
	movieIDStr := r.URL.Query().Get("movie_id")
	if movieIDStr == "" {
		http.Error(w, "Movie ID is required", http.StatusBadRequest)
		return
	}

	movieID, err := strconv.Atoi(movieIDStr)
	if err != nil {
		http.Error(w, "Invalid movie ID", http.StatusBadRequest)
		return
	}

	// In a real app, we would fetch movie details and available showtimes from the database
	// For demonstration, we'll use hardcoded data
	movie := Movie{
		ID:       movieID,
		Title:    "Sikander",
		Duration: 177,
		Rating:   "8.5",
	}

	// Create a dummy showtime
	showtime := Showtime{
		ID:        1,
		MovieID:   movieID,
		DateTime:  time.Now().Add(time.Hour * 3),
		Time:      time.Now().Add(time.Hour * 3),
		Theater:   "PVR Cinemas",
		Screen:    "Screen 1",
		Hall:      "Hall 1",
		Price:     250,
		Available: 120,
		Format:    "2D",
		Formats:   []string{"2D", "Dolby Atmos"},
		Movie:     movie,
	}

	// Lets assume these seats are already booked
	bookedSeats := []int{3, 7, 12, 18, 24, 35, 42, 56}

	// Create a seat layout
	// Platinum rows (1-4)
	platinumRowCount := 4
	platinumSeatsPerRow := 12

	// Gold rows (5-10)
	goldRowCount := 6
	goldSeatsPerRow := 16

	// Generate seat rows for platinum section
	platinumRows := make([][]int, platinumRowCount)
	seatNumber := 1
	for i := 0; i < platinumRowCount; i++ {
		platinumRows[i] = make([]int, platinumSeatsPerRow)
		for j := 0; j < platinumSeatsPerRow; j++ {
			platinumRows[i][j] = seatNumber
			seatNumber++
		}
	}

	// Generate seat rows for gold section
	goldRows := make([][]int, goldRowCount)
	for i := 0; i < goldRowCount; i++ {
		goldRows[i] = make([]int, goldSeatsPerRow)
		for j := 0; j < goldSeatsPerRow; j++ {
			goldRows[i][j] = seatNumber
			seatNumber++
		}
	}

	// Generate row labels
	rowLabels := make([]string, platinumRowCount+goldRowCount)
	for i := 0; i < platinumRowCount; i++ {
		rowLabels[i] = string('A' + i)
	}
	for i := 0; i < goldRowCount; i++ {
		rowLabels[platinumRowCount+i] = string('E' + i)
	}

	// Prepare data for template
	data := struct {
		Showtime            Showtime
		PlatinumRows        [][]int
		PlatinumSeatsPerRow int
		GoldRows            [][]int
		GoldSeatsPerRow     int
		RowLabels           []string
		BookedSeats         []int
		PlatinumPrice       float64
		RegularPrice        float64
	}{
		Showtime:            showtime,
		PlatinumRows:        platinumRows,
		PlatinumSeatsPerRow: platinumSeatsPerRow,
		GoldRows:            goldRows,
		GoldSeatsPerRow:     goldSeatsPerRow,
		RowLabels:           rowLabels,
		BookedSeats:         bookedSeats,
		PlatinumPrice:       350.0, // Premium pricing for platinum seats
		RegularPrice:        250.0, // Regular pricing for gold seats
	}

	// Define template functions
	funcMap := template.FuncMap{
		"isSeatBooked": isSeatBooked,
		"add":          add,
	}

	// Parse and execute template
	tmpl, err := template.New("select_seats.gohtml").Funcs(funcMap).ParseFiles("templates/select_seats.gohtml")
	if err != nil {
		http.Error(w, "Failed to parse template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to render template: "+err.Error(), http.StatusInternalServerError)
	}
}

// Get showtime with movie details
func getShowtimeWithMovie(showtimeID int) (Showtime, error) {
	var showtime Showtime
	var seatsJSON sql.NullString
	var languagesJSON, formatsJSON sql.NullString
	var posterURL, backdropURL sql.NullString
	var releaseDateStr sql.NullString

	query := `SELECT s.id, s.movie_id, s.date_time, s.hall, s.price, s.available, s.seats, s.format,
			  m.id, m.title, m.description, m.duration, m.genre, m.director, m.cast,
			  m.rating, m.vote_count, m.poster_url, m.backdrop_url, m.release_date,
			  m.languages, m.formats, m.is_upcoming
			  FROM showtimes s
			  JOIN movies m ON s.movie_id = m.id
			  WHERE s.id = ?`

	err := db.QueryRow(query, showtimeID).Scan(
		&showtime.ID, &showtime.MovieID, &showtime.DateTime, &showtime.Hall,
		&showtime.Price, &showtime.Available, &seatsJSON, &showtime.Format,
		&showtime.Movie.ID, &showtime.Movie.Title, &showtime.Movie.Description,
		&showtime.Movie.Duration, &showtime.Movie.Genre, &showtime.Movie.Director,
		&showtime.Movie.Cast, &showtime.Movie.Rating, &showtime.Movie.VoteCount,
		&posterURL, &backdropURL, &releaseDateStr,
		&languagesJSON, &formatsJSON, &showtime.Movie.IsUpcoming)

	if err != nil {
		return showtime, err
	}

	// Parse seats
	if seatsJSON.Valid {
		json.Unmarshal([]byte(seatsJSON.String), &showtime.Seats)
	}

	// Set nullable fields
	showtime.Movie.PosterURL = posterURL
	showtime.Movie.BackdropURL = backdropURL

	// Parse the release date string into a time.Time
	if releaseDateStr.Valid {
		releaseDate, err := time.Parse("2006-01-02", releaseDateStr.String)
		if err == nil {
			showtime.Movie.ReleaseDate.Valid = true
			showtime.Movie.ReleaseDate.Time = releaseDate
		}
	}

	// Parse languages and formats
	if languagesJSON.Valid {
		json.Unmarshal([]byte(languagesJSON.String), &showtime.Movie.Languages)
	}
	if formatsJSON.Valid {
		json.Unmarshal([]byte(formatsJSON.String), &showtime.Movie.Formats)
	}

	return showtime, nil
}

// Handle the payment page
func handlePayment(w http.ResponseWriter, r *http.Request) {
	// Check if this is a form submission (POST) or initial page load (GET)
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var showtimeID string
	var selectedSeatsStr string

	if r.Method == http.MethodPost {
		// Form submission
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		showtimeID = r.FormValue("showtime_id")
		selectedSeatsStr = r.FormValue("selected_seats")
	} else {
		// Initial page load (GET)
		showtimeID = r.URL.Query().Get("showtime_id")
		selectedSeatsStr = r.URL.Query().Get("seats")
	}

	// Basic validation
	if showtimeID == "" || selectedSeatsStr == "" {
		http.Error(w, "Missing required parameters: showtime_id and seats", http.StatusBadRequest)
		return
	}

	// Parse showtime ID
	showtimeIDInt, err := strconv.Atoi(showtimeID)
	if err != nil {
		http.Error(w, "Invalid showtime ID", http.StatusBadRequest)
		return
	}

	// Parse selected seats
	var selectedSeats []int
	for _, seatStr := range strings.Split(selectedSeatsStr, ",") {
		seatNum, err := strconv.Atoi(seatStr)
		if err != nil {
			continue // Skip invalid seat numbers
		}
		selectedSeats = append(selectedSeats, seatNum)
	}

	if len(selectedSeats) == 0 {
		http.Error(w, "No valid seats selected", http.StatusBadRequest)
		return
	}

	// Create a dummy showtime for demonstration
	movie := Movie{
		ID:       3,
		Title:    "Sikander",
		Duration: 177,
		Rating:   "8.5",
	}

	showtime := Showtime{
		ID:        showtimeIDInt,
		MovieID:   3, // Example movie ID
		DateTime:  time.Now().Add(time.Hour * 3),
		Time:      time.Now().Add(time.Hour * 3),
		Theater:   "PVR Cinemas",
		Screen:    "Screen 1",
		Hall:      "Hall 1",
		Price:     250,
		Available: 120,
		Format:    "2D",
		Formats:   []string{"2D", "Dolby Atmos"},
		Movie:     movie,
	}

	// Count platinum vs regular seats for pricing
	platinumSeats := 0
	regularSeats := 0

	// Platinum seats are 1-48 (4 rows x 12 seats)
	platinumMax := 48

	for _, seat := range selectedSeats {
		if seat <= platinumMax {
			platinumSeats++
		} else {
			regularSeats++
		}
	}

	// Calculate prices
	platinumPrice := 350.0
	regularPrice := 250.0
	seatsCost := platinumPrice*float64(platinumSeats) + regularPrice*float64(regularSeats)
	convenienceFee := 30.0 * float64(len(selectedSeats))
	totalAmount := seatsCost + convenienceFee

	// Generate seat labels
	seatLabels := generateSeatLabels(selectedSeats)

	// Create a unique booking ID
	bookingID := fmt.Sprintf("BK%d%d", showtime.ID, time.Now().Unix())

	// Prepare data for the payment template
	data := struct {
		BookingID           string
		Showtime            Showtime
		Movie               Movie // Add separate Movie field for the template
		SelectedSeats       []int
		SeatLabels          []string
		PlatinumSeats       int
		RegularSeats        int
		PlatinumPrice       float64
		RegularPrice        float64
		SeatsCost           float64
		ConvenienceFee      float64
		TotalAmount         float64
		TimeRemaining       int // In minutes
		SelectedSeatsString string
	}{
		BookingID:           bookingID,
		Showtime:            showtime,
		Movie:               movie, // Set the Movie field
		SelectedSeats:       selectedSeats,
		SeatLabels:          seatLabels,
		PlatinumSeats:       platinumSeats,
		RegularSeats:        regularSeats,
		PlatinumPrice:       platinumPrice,
		RegularPrice:        regularPrice,
		SeatsCost:           seatsCost,
		ConvenienceFee:      convenienceFee,
		TotalAmount:         totalAmount,
		TimeRemaining:       10, // 10 minutes to complete payment
		SelectedSeatsString: selectedSeatsStr,
	}

	// Parse and execute the payment template
	tmpl, err := template.ParseFiles("templates/payment.gohtml")
	if err != nil {
		http.Error(w, "Failed to parse payment template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to render payment template: "+err.Error(), http.StatusInternalServerError)
	}
}

// Generate human-readable seat labels from seat numbers
func generateSeatLabels(seatNumbers []int) []string {
	var labels []string

	for _, seatNum := range seatNumbers {
		// For the first 4 rows (Platinum - rows A-D)
		if seatNum <= 48 { // 4 rows of 12 seats
			rowChar := 'A' + rune((seatNum-1)/12)
			seatInRow := (seatNum-1)%12 + 1
			labels = append(labels, fmt.Sprintf("%c%d", rowChar, seatInRow))
		} else { // Gold section - rows E-J
			adjustedSeatNum := seatNum - 48 // Adjust to start from 1 after platinum section
			rowChar := 'E' + rune((adjustedSeatNum-1)/16)
			seatInRow := (adjustedSeatNum-1)%16 + 1
			labels = append(labels, fmt.Sprintf("%c%d", rowChar, seatInRow))
		}
	}

	return labels
}

// Handle booking confirmation after payment
func handleBookingConfirm(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Get data from form
	showtimeID := r.FormValue("showtime_id")
	selectedSeatsStr := r.FormValue("selected_seats")
	totalAmountStr := r.FormValue("total_amount")
	customerName := r.FormValue("customer_name")
	bookingID := r.FormValue("booking_id")

	// Get optional fields with fallbacks
	customerEmail := r.FormValue("customer_email")
	if customerEmail == "" {
		customerEmail = "N/A"
	}

	customerPhone := r.FormValue("customer_phone")
	if customerPhone == "" {
		customerPhone = "N/A"
	}

	paymentMethod := r.FormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "card"
	}

	// Basic validation
	if showtimeID == "" || selectedSeatsStr == "" || totalAmountStr == "" || customerName == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Parse showtime ID
	showtimeIDInt, err := strconv.Atoi(showtimeID)
	if err != nil {
		http.Error(w, "Invalid showtime ID", http.StatusBadRequest)
		return
	}

	// Parse total amount
	totalAmount, err := strconv.ParseFloat(totalAmountStr, 64)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	// Parse selected seats
	var selectedSeats []int
	for _, seatStr := range strings.Split(selectedSeatsStr, ",") {
		seatNum, err := strconv.Atoi(seatStr)
		if err != nil {
			continue // Skip invalid seat numbers
		}
		selectedSeats = append(selectedSeats, seatNum)
	}

	// Create dummy showtime data
	showtime := Showtime{
		ID:        showtimeIDInt,
		MovieID:   3, // Example movie ID
		DateTime:  time.Now().Add(time.Hour * 3),
		Time:      time.Now().Add(time.Hour * 3),
		Theater:   "PVR Cinemas",
		Screen:    "Screen 1",
		Hall:      "Hall 1",
		Price:     250,
		Available: 120,
		Format:    "2D",
		Formats:   []string{"2D", "Dolby Atmos"},
		Movie: Movie{
			ID:       3,
			Title:    "Sikander",
			Duration: 177,
			Rating:   "8.5",
		},
	}

	// Generate seat labels
	seatLabels := generateSeatLabels(selectedSeats)

	// Calculate convenience fee and base price (for display)
	convenienceFee := 30.0 * float64(len(selectedSeats))
	basePrice := (totalAmount - convenienceFee) / float64(len(selectedSeats))

	// Create a transaction ID
	transactionID := fmt.Sprintf("TXN%d%d", showtimeIDInt, time.Now().Unix())

	// In a real application, we would save the booking to database here

	// Prepare data for confirmation template
	data := struct {
		BookingID      string
		TransactionID  string
		Showtime       Showtime
		SelectedSeats  []int
		SeatLabels     []string
		PaymentMethod  string
		CustomerName   string
		CustomerEmail  string
		CustomerPhone  string
		BasePrice      float64
		ConvenienceFee float64
		TotalAmount    float64
		BookingTime    time.Time
	}{
		BookingID:      bookingID,
		TransactionID:  transactionID,
		Showtime:       showtime,
		SelectedSeats:  selectedSeats,
		SeatLabels:     seatLabels,
		PaymentMethod:  paymentMethod,
		CustomerName:   customerName,
		CustomerEmail:  customerEmail,
		CustomerPhone:  customerPhone,
		BasePrice:      basePrice,
		ConvenienceFee: convenienceFee,
		TotalAmount:    totalAmount,
		BookingTime:    time.Now(),
	}

	// Render the confirmation template
	tmpl, err := template.ParseFiles("templates/confirmation.gohtml")
	if err != nil {
		http.Error(w, "Failed to parse confirmation template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to render confirmation template: "+err.Error(), http.StatusInternalServerError)
	}
}

func handleMyBookings(w http.ResponseWriter, r *http.Request) {
	// Get bookings from database
	rows, err := db.Query(`
		SELECT b.id, b.showtime_id, b.customer, b.email, b.phone, 
			   b.seats, b.total_price, b.status, b.created_at,
			   s.date_time, s.hall, s.format,
			   m.title, m.poster_url
		FROM bookings b
		JOIN showtimes s ON b.showtime_id = s.id
		JOIN movies m ON s.movie_id = m.id
		ORDER BY b.created_at DESC
		LIMIT 20`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		var seatsJSON string
		var posterURL sql.NullString
		var emailNull, phoneNull sql.NullString // Use NullString for potentially NULL fields
		var createdAtStr sql.NullString         // Use NullString for timestamp
		var dateTimeStr sql.NullString          // Use NullString for date_time field

		err := rows.Scan(&b.ID, &b.ShowtimeID, &b.Customer, &emailNull, &phoneNull,
			&seatsJSON, &b.TotalPrice, &b.Status, &createdAtStr,
			&dateTimeStr, &b.Showtime.Hall, &b.Showtime.Format,
			&b.Showtime.Movie.Title, &posterURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set values from nullable fields
		if emailNull.Valid {
			b.Email = emailNull.String
		}

		if phoneNull.Valid {
			b.Phone = phoneNull.String
		}

		// Parse created_at timestamp
		if createdAtStr.Valid {
			createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr.String)
			if err == nil {
				b.CreatedAt = createdAt
			} else {
				// Try alternate format
				createdAt, err = time.Parse(time.RFC3339, createdAtStr.String)
				if err == nil {
					b.CreatedAt = createdAt
				}
			}
		}

		// Parse date_time timestamp
		if dateTimeStr.Valid {
			dateTime, err := time.Parse("2006-01-02 15:04:05", dateTimeStr.String)
			if err == nil {
				b.Showtime.DateTime = dateTime
				b.Showtime.Time = dateTime // Also set the Time field
			} else {
				// Try alternate format
				dateTime, err = time.Parse(time.RFC3339, dateTimeStr.String)
				if err == nil {
					b.Showtime.DateTime = dateTime
					b.Showtime.Time = dateTime
				}
			}
		}

		b.Showtime.Movie.PosterURL = posterURL
		json.Unmarshal([]byte(seatsJSON), &b.Seats)
		bookings = append(bookings, b)
	}

	data := struct {
		Bookings []Booking
	}{
		Bookings: bookings,
	}

	err = tpl.ExecuteTemplate(w, "bookings.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleMoviesAPI(w http.ResponseWriter, r *http.Request) {
	// Implementation of handleMoviesAPI function
}

func handleMovieAPI(w http.ResponseWriter, r *http.Request) {
	// Implementation of handleMovieAPI function
}

func handleShowtimesAPI(w http.ResponseWriter, r *http.Request) {
	// Implementation of handleShowtimesAPI function
}

func generateDateOptions() []DateOption {
	days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	var dates []DateOption
	now := time.Now()

	for i := 0; i < 7; i++ {
		date := now.AddDate(0, 0, i)
		dates = append(dates, DateOption{
			Date: date.Format("2"),
			Day:  days[date.Weekday()],
		})
	}

	return dates
}

func getShowtimes(movieID int) ([]Showtime, error) {
	query := `SELECT s.id, s.movie_id, s.date_time, s.hall, s.price, s.available, 
			  s.seats, s.format FROM showtimes s 
			  WHERE s.movie_id = ? AND s.date_time >= NOW() 
			  ORDER BY s.date_time LIMIT 20`

	rows, err := db.Query(query, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var showtimes []Showtime
	for rows.Next() {
		var s Showtime
		var seatsJSON sql.NullString
		err := rows.Scan(&s.ID, &s.MovieID, &s.DateTime, &s.Hall, &s.Price,
			&s.Available, &seatsJSON, &s.Format)
		if err != nil {
			return nil, err
		}

		if seatsJSON.Valid {
			json.Unmarshal([]byte(seatsJSON.String), &s.Seats)
		}

		showtimes = append(showtimes, s)
	}

	return showtimes, nil
}

// Admin Dashboard Handler
func handleAdmin(w http.ResponseWriter, r *http.Request) {
	// Get stats for dashboard
	var totalMovies, totalShowtimes, totalBookings int
	var totalRevenue float64

	// Count total movies
	err := db.QueryRow("SELECT COUNT(*) FROM movies").Scan(&totalMovies)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Count active showtimes (future showtimes)
	err = db.QueryRow("SELECT COUNT(*) FROM showtimes WHERE date_time > NOW()").Scan(&totalShowtimes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Count total bookings
	err = db.QueryRow("SELECT COUNT(*) FROM bookings").Scan(&totalBookings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate total revenue
	err = db.QueryRow("SELECT COALESCE(SUM(total_price), 0) FROM bookings").Scan(&totalRevenue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		TotalMovies    int
		TotalShowtimes int
		TotalBookings  int
		TotalRevenue   string
	}{
		TotalMovies:    totalMovies,
		TotalShowtimes: totalShowtimes,
		TotalBookings:  totalBookings,
		TotalRevenue:   fmt.Sprintf("%.2f", totalRevenue),
	}

	err = tpl.ExecuteTemplate(w, "admin.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Add Movie Handler
func handleAddMovie(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Display the form
		data := struct {
			Error   string
			Success string
		}{
			Error:   r.URL.Query().Get("error"),
			Success: r.URL.Query().Get("success"),
		}
		err := tpl.ExecuteTemplate(w, "add_movie.gohtml", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Process the form submission
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape("Failed to parse form"), http.StatusSeeOther)
			return
		}

		// Get form values
		title := r.FormValue("title")
		description := r.FormValue("description")
		durationStr := r.FormValue("duration")
		genre := r.FormValue("genre")
		director := r.FormValue("director")
		cast := r.FormValue("cast")
		rating := r.FormValue("rating")
		releaseDate := r.FormValue("release_date")
		posterURL := r.FormValue("poster_url")
		backdropURL := r.FormValue("backdrop_url")
		isUpcoming := r.FormValue("is_upcoming") == "true"

		// Validate required fields
		if title == "" || durationStr == "" {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape("Title and duration are required"), http.StatusSeeOther)
			return
		}

		// Parse duration
		duration, err := strconv.Atoi(durationStr)
		if err != nil || duration <= 0 {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape("Invalid duration"), http.StatusSeeOther)
			return
		}

		// Process languages (checkboxes)
		languages := r.Form["languages"]
		languagesJSON, err := json.Marshal(languages)
		if err != nil {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape("Invalid languages"), http.StatusSeeOther)
			return
		}

		// Process formats (checkboxes)
		formats := r.Form["formats"]
		formatsJSON, err := json.Marshal(formats)
		if err != nil {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape("Invalid formats"), http.StatusSeeOther)
			return
		}

		// Prepare SQL query
		query := `INSERT INTO movies (
			title, description, duration, genre, director, cast, rating, 
			poster_url, backdrop_url, release_date, languages, formats, is_upcoming
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

		// Execute the query
		_, err = db.Exec(query, title, description, duration, genre, director, cast, rating,
			posterURL, backdropURL, releaseDate, string(languagesJSON), string(formatsJSON), isUpcoming)
		if err != nil {
			http.Redirect(w, r, "/admin/movie/add?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}

		// Redirect with success message
		http.Redirect(w, r, "/admin/movie/add?success="+url.QueryEscape("Movie added successfully"), http.StatusSeeOther)
		return
	}

	// Method not allowed
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// Add Showtime Handler
func handleAddShowtime(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Get all movies for the dropdown
		movies, err := getMovies(false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Add upcoming movies as well
		upcomingMovies, err := getMovies(true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Combine both lists
		allMovies := append(movies, upcomingMovies...)

		data := struct {
			Movies  []Movie
			Error   string
			Success string
		}{
			Movies:  allMovies,
			Error:   r.URL.Query().Get("error"),
			Success: r.URL.Query().Get("success"),
		}

		err = tpl.ExecuteTemplate(w, "add_showtime.gohtml", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Process the form submission
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("Failed to parse form"), http.StatusSeeOther)
			return
		}

		// Get form values
		movieIDStr := r.FormValue("movie_id")
		dateStr := r.FormValue("date")
		timeStr := r.FormValue("time")
		hall := r.FormValue("hall")
		format := r.FormValue("format")
		priceStr := r.FormValue("price")
		availableStr := r.FormValue("available")

		// Validate required fields
		if movieIDStr == "" || dateStr == "" || timeStr == "" || hall == "" || format == "" || priceStr == "" || availableStr == "" {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("All fields are required"), http.StatusSeeOther)
			return
		}

		// Parse movie ID
		movieID, err := strconv.Atoi(movieIDStr)
		if err != nil {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("Invalid movie ID"), http.StatusSeeOther)
			return
		}

		// Parse date and time
		dateTime, err := time.Parse("2006-01-02 15:04", dateStr+" "+timeStr)
		if err != nil {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("Invalid date or time"), http.StatusSeeOther)
			return
		}

		// Parse price
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil || price <= 0 {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("Invalid price"), http.StatusSeeOther)
			return
		}

		// Parse available seats
		available, err := strconv.Atoi(availableStr)
		if err != nil || available <= 0 {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape("Invalid number of available seats"), http.StatusSeeOther)
			return
		}

		// Format the date time for MySQL
		formattedDateTime := dateTime.Format("2006-01-02 15:04:05")

		// Prepare SQL query
		query := `INSERT INTO showtimes (movie_id, date_time, hall, price, available, format) 
				  VALUES (?, ?, ?, ?, ?, ?)`

		// Execute the query
		_, err = db.Exec(query, movieID, formattedDateTime, hall, price, available, format)
		if err != nil {
			http.Redirect(w, r, "/admin/showtime/add?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}

		// Redirect with success message
		http.Redirect(w, r, "/admin/showtime/add?success="+url.QueryEscape("Showtime added successfully"), http.StatusSeeOther)
		return
	}

	// Method not allowed
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
