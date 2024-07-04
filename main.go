package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/joho/godotenv"
)

type ExchangeType string

const (
	NYSE   ExchangeType = "NYSE"
	NASDAQ ExchangeType = "NASDAQ"
	TSX    ExchangeType = "TSX"
)

type Stock struct {
	ID          uint         `gorm:"column:id;primary_key"`
	Ticker      string       `gorm:"column:ticker;not null"`
	CompanyName string       `gorm:"column:company_name"`
	Exchange    ExchangeType `gorm:"not null"`
	IsActive    bool         `gorm:"column:is_active;not null;default:true"`
	CreatedAt   time.Time    `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time    `gorm:"column:updated_at;not null"`
}

func (Stock) TableName() string {
	return "stocks"
}

type EarningsDate struct {
	ID               uint      `gorm:"column:id;primary_key"`
	StockID          uint      `gorm:"column:stock_id;not null"`
	EarningsDateTime time.Time `gorm:"column:earnings_datetime;not null"`
}

func (EarningsDate) TableName() string {
	return "earnings_dates"
}

type ErrorLevel string

const (
	Debug ErrorLevel = "DEBUG"
	Info  ErrorLevel = "INFO"
	Warn  ErrorLevel = "WARN"
	Error ErrorLevel = "ERROR"
	Fatal ErrorLevel = "FATAL"
)

type DbLogger struct {
	ID        uint       `gorm:"column:id;primary_key"`
	Timestamp time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
	Level     ErrorLevel `gorm:"not null"`
	Message   string     `gorm:"not null"`
	StockID   *uint      `gorm:"column:stock_id"`
}

func (DbLogger) TableName() string {
	return "logs"
}

func writeDbLog(db *gorm.DB, level ErrorLevel, message string, stockID *uint) error {
	logger := DbLogger{
		Level:   level,
		Message: message,
		StockID: stockID,
	}

	if logger.Timestamp.IsZero() {
		logger.Timestamp = time.Now()
	}

	// Ensure that there are at most 1000 rows in the logs table
	var logCount int
	if err := db.Model(&DbLogger{}).Count(&logCount).Error; err != nil {
		return err
	}
	if logCount >= 1000 {
		// Delete the oldest log
		if err := db.Where("id = (SELECT MIN(id) FROM logs)").Delete(&DbLogger{}).Error; err != nil {
			return err
		}
	}

	if err := db.Create(&logger).Error; err != nil {
		return err
	}

	return nil
}

func truncateFile(file *os.File) error {
	maxLines := 1000

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("Failed to get file stat: %w", err)
	}

	fileSize := stat.Size()
	lineCount := 0
	bufferSize := 1024 // 1KB

	for lineCount < maxLines && fileSize > 0 {
		readSize := int64(bufferSize)
		if readSize > fileSize {
			readSize = fileSize
		}

		buffer := make([]byte, readSize)
		_, err := file.ReadAt(buffer, fileSize-readSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("Failed to read log file: %w", err)
		}

		for i := len(buffer) - 1; i >= 0; i-- {
			if buffer[i] == '\n' {
				lineCount++
				if lineCount >= maxLines {
					break
				}
			}
		}

		fileSize -= readSize
	}

	if lineCount >= maxLines {
		return file.Truncate(fileSize)
	}

	return nil
}

func writeFileLog(level ErrorLevel, message string, stockID *uint) error {
	type FileLogger struct {
		Level   ErrorLevel
		Message string
		StockID *uint
	}

	logger := FileLogger{
		Level:   level,
		Message: message,
		StockID: stockID,
	}

	logFile, err := os.OpenFile("log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("Unable to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)

	stockIDStr := "nil"
	if logger.StockID != nil {
		stockIDStr = strconv.FormatUint(uint64(*logger.StockID), 10)
	}

	msg := "[" + string(logger.Level) + "] " + logger.Message
	if logger.StockID != nil {
		msg += " (stock_id: " + stockIDStr + ")"
	}

	log.Println(msg)

	// Truncate log file
	err = truncateFile(logFile)
	if err != nil {
		return fmt.Errorf("Failed to truncate log file: %w", err)
	}

	return nil
}

func scrapeEarningsDates(debugFlag bool) {
	_ = writeFileLog(Info, "Starting earnings date scraping...", nil)

	if debugFlag {
		fmt.Println("Debug mode enabled")
	}

	err := godotenv.Load()
	if err != nil {
		// Could not load .env file
		fmt.Println("Could not load .env file: " + err.Error())
		_ = writeFileLog(Fatal, "Could not load .env file: "+err.Error(), nil)
		return
	}

	dbDatabase := os.Getenv("DB_DATABASE")
	dbHost := os.Getenv("DB_HOSTNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USERNAME")

	dbURI := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbDatabase, dbPassword)

	db, err := gorm.Open("postgres", dbURI)
	if err != nil {
		_ = writeFileLog(Fatal, "Could not connect to the database: "+err.Error(), nil)
	}
	defer db.Close()

	// Check if we have more than 10 errors in the loggers table
	var dbErrorCount int
	err = db.Model(&DbLogger{}).Where("level = ?", Error).Count(&dbErrorCount).Error
	if err != nil {
		err := writeFileLog(Fatal, "Could not count errors in the loggers table: "+err.Error(), nil)
		if err != nil {
			fmt.Println(err)
		}

		return
	}

	if dbErrorCount > 9 {
		// Too many errors in the loggers table
		err := writeFileLog(Error, "Too many error logs exist in the loggers table. Exiting until next time", nil)
		if err != nil {
			fmt.Println(err)
		}

		return
	}

	var stock Stock
	err = db.Where("is_active = ?", true).Order("updated_at ASC").First(&stock).Error
	if err != nil {
		err := writeFileLog(Fatal, "Could not find an active stock: "+err.Error(), nil)
		if err != nil {
			fmt.Println(err)
		}

		return
	}

	userAgents := []string{
		// "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.98 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.2592.87",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:127.0) Gecko/20100101 Firefox/127.0",
	}
	userAgent := userAgents[time.Now().UnixNano()%int64(len(userAgents))]

	client := &http.Client{}
	url := "https://finance.yahoo.com/calendar/earnings?symbol=" + stock.Ticker
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		if err := writeDbLog(db, Error, "Could not create new HTTP request: "+err.Error(), &stock.ID); err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Upgrade-Insecure-Requests", "1")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")

	response, err := client.Do(request)
	if err != nil {
		if err := writeDbLog(db, Error, "Could not execute the request: "+err.Error(), &stock.ID); err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		if err := writeDbLog(db, Error, "Yahoo returned non-200 status code: "+strconv.FormatUint(uint64(stock.ID), 10), &stock.ID); err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}
		_ = writeFileLog(Error, "Yahoo returned non-200 status code: "+strconv.FormatUint(uint64(stock.ID), 10), &stock.ID)

		return
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		if err := writeDbLog(db, Error, "Could not create new document from response body: "+err.Error(), &stock.ID); err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}

	companyName := strings.TrimSpace(doc.Find("td[aria-label='Company']").First().Text())

	if companyName == "" {
		// We couldn't find the company name, so we'll mark the stock as inactive
		db.Model(&stock).Update("is_active", false)

		err := writeDbLog(db, Warn, fmt.Sprintf("Could not find company name. Marked as inactive: %s", stock.Ticker), &stock.ID)
		if err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}

	earningsDatesDomElement := doc.Find("td[aria-label='Earnings Date']")
	if earningsDatesDomElement.Length() == 0 {
		err := writeDbLog(db, Error, fmt.Sprintf("Could not find earnings dates for ticker: %s", stock.Ticker), &stock.ID)
		if err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}

	var discoveredEarningsDates []time.Time
	earningsDatesDomElement.Each(func(i int, s *goquery.Selection) {
		earningsDateRaw := strings.TrimSpace(s.Text())

		// Check the length of the earnings date string to ensure it's what we are
		// looking for. (We are scraping after all..)
		if len(earningsDateRaw) > 4 {
			// Check if a timezone suffix exists on the string. If not, append UTC.
			hasNyTimezone := strings.HasSuffix(earningsDateRaw, "EDT") || strings.HasSuffix(earningsDateRaw, "EST")
			loc, _ := time.LoadLocation("America/New_York")

			if hasNyTimezone {
				// Ensure there is a space between each time and timezone
				earningsDateRaw = strings.ReplaceAll(earningsDateRaw, "AM", "AM ")
				earningsDateRaw = strings.ReplaceAll(earningsDateRaw, "PM", "PM ")
			} else {
				loc, _ = time.LoadLocation("UTC")
				earningsDateRaw = strings.ReplaceAll(earningsDateRaw, "UTC", " UTC")
			}

			dt, err := time.ParseInLocation("Jan 02, 2006, 3 PM MST", earningsDateRaw, loc)
			if err != nil {
				err := writeDbLog(db, Error, "Error parsing earnings date"+err.Error(), &stock.ID)
				if err != nil {
					_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
				}

				return
			}

			// Convert to UTC
			utcTime := dt.UTC()

			discoveredEarningsDates = append(discoveredEarningsDates, utcTime)
		}
	})

	var currentEarningsDates []EarningsDate
	if err := db.Where("stock_id = ?", stock.ID).Find(&currentEarningsDates).Error; err != nil {
		err := writeDbLog(db, Error, "Could not find earnings dates for stock "+err.Error(), &stock.ID)
		if err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		return
	}

	// Create a map of discovered earnings dates for quick lookup
	discoveredEarningsDatesMap := make(map[time.Time]bool)
	for _, date := range discoveredEarningsDates {
		discoveredEarningsDatesMap[date] = true
	}

	// Counters for added and removed dates
	addedCount := 0
	removedCount := 0
	ohShit := false

	// Remove dates that are no longer present in the discovered dates
	for _, currentDate := range currentEarningsDates {
		if !discoveredEarningsDatesMap[currentDate.EarningsDateTime] {
			if err := db.Delete(&currentDate).Error; err != nil {
				err := writeDbLog(db, Error, "Could not delete old earnings date: "+err.Error(), &stock.ID)
				if err != nil {
					_ = writeFileLog(Error, "Could not write to the database (inside loop): "+err.Error(), &stock.ID)
				}
				if !ohShit {
					ohShit = true
				}
			} else {
				removedCount++
			}
		}
	}

	// Add dates that are present in the discovered dates but not in the current dates
	currentEarningsDatesMap := make(map[time.Time]bool)
	for _, date := range currentEarningsDates {
		currentEarningsDatesMap[date.EarningsDateTime.UTC()] = true
	}

	for _, discoveredDate := range discoveredEarningsDates {
		if !currentEarningsDatesMap[discoveredDate] {
			newEarningsDate := EarningsDate{
				StockID:          stock.ID,
				EarningsDateTime: discoveredDate,
			}
			if err := db.Create(&newEarningsDate).Error; err != nil {
				err := writeDbLog(db, Error, "Could not add new earnings date"+err.Error(), &stock.ID)
				if err != nil {
					_ = writeFileLog(Error, "Could not write to the database (inside loop): "+err.Error(), &stock.ID)
				}
				if !ohShit {
					ohShit = true
				}
			} else {
				addedCount++
			}
		}
	}

	if ohShit {
		err := writeDbLog(db, Error, "Something went wrong while updating earnings dates", &stock.ID)
		if err != nil {
			_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		}

		_ = writeFileLog(Error, "Something went wrong while updating earnings dates: "+err.Error(), &stock.ID)

		// Delete all earnings dates for this stock
		// if err := db.Where("stock_id = ?", stock.ID).Delete(&EarningsDate{}).Error; err != nil {
		// 	err := writeDbLog(db, Error, "Could not delete all earnings dates for stock", &stock.ID)
		// 	if err != nil {
		// 		_ = writeFileLog(Error, "Could not write to the database: "+err.Error(), &stock.ID)
		// 	}
		// }
	}

	if debugFlag {
		fmt.Println(time.Now())
		fmt.Println("Stock ID: ", stock.ID)
		fmt.Println("Ticker: ", stock.Ticker)
		// Print the number of added and removed dates - for debugging purposes
		fmt.Printf("Number of added earnings dates: %d\n", addedCount)
		fmt.Printf("Number of removed earnings dates: %d\n", removedCount)
		fmt.Println("-----------------------------------")
	}

	// Update the stock record with the new company name and updated_at timestamp.
	db.Model(&stock).Update("company_name", companyName)

	if !ohShit {
		db.Model(&stock).Update("updated_at", time.Now())
		_ = writeFileLog(Info, "Earnings date scraping completed successfully", &stock.ID)
	}

	// return // redundant return
}

func hasNetworkConnection() bool {
	// Try a DNS lookup on a very well known domain
	_, err := net.LookupHost("google.com")
	return err == nil
}

func main() {
	_ = writeFileLog(Info, "Earnings scraper initialized...", nil)

	debugFlag := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()

	// Run the scraping function initially
	if hasNetworkConnection() {
		scrapeEarningsDates(*debugFlag)
	}

	timer := time.NewTicker(time.Minute)

	// Run the scraping function every minute
	for {
		<-timer.C

		if !hasNetworkConnection() {
			_ = writeFileLog(Fatal, "No network connection detected", nil)
			continue
		}

		scrapeEarningsDates(*debugFlag)
	}
}
