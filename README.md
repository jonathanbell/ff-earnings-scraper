# Focus Financial Earnings Dates Scraper

Go program to scrape stock earnings dates data. Intended to be setup as a service.

## Usage/installation

1. Download the compiled executable file and place it inside a folder inside your home directory (example: `~/ff-earnings-dates-scraper/ff_scrape_earnings-win64.exe`).
1. Place a file called `.env` next to the executable.
1. Inside `.env` place all of your database configuration and secrets. The file should have the following keys (with their values set):

```plaintext
DB_HOSTNAME=
DB_USERNAME=
DB_PASSWORD=
DB_DATABASE=
DB_PORT=
```

While developing you can pass the `-debug` flag for a little more verbose output when running the script (`go run main.go -debug`).

### Install the program as a service on Windows

1. Open the Windows Service Manager:
   - Press `Win + R`, type `services.msc`, and hit Enter.
2. Add a new service:
   - Right-click in the Services list and select "New" -> "Service".
3. Service Information:
   - Service name: Provide a meaningful name for your service (e.g., "FF Earnings Data Scraper").
   - Description (optional):  Enter a brief description.
   - Startup type: Choose "Automatic" if you want the service to start automatically on boot or "Manual" to start it manually.
4. Browse for your executable:
   - Click "Browse..." and locate your compiled `.exe` file.
5. Specify service account (optional, but recommended):
   - If your service needs specific permissions to access files, databases, or network resources, create a dedicated user account in Windows with those permissions.
   - Click "This account..." and enter the credentials for that account.
6. Finish and start:
   - Click "OK" to create the service.
   - You can now start, stop, and manage the service like any other Windows service from the Service Manager.

#### Important considerations

- Working Directory: The service will execute in the `System32` directory by default. However  `ff_scrape_earnings-win64.exe` relies on the user's home directory, so you'll need to set the working directory explicitly:
  - Go back to the service properties in Service Manager.
  - On the "General" tab, click "Start parameters" and enter the following, replacing `C:\user\home\path\ff-earnings-dates-scraper` with the correct path: `/d "C:\user\home\path\ff-earnings-dates-scraper"`

### Install the program as a service on Mac OS X

1. First, make a directory: `mkdir -p ~/ff-earnings-dates-scraper`
1. Place the executable file inside `~/ff-earnings-dates-scraper`
1. Make executable: `chmod +x ~/ff-earnings-dates-scraper/ff_scrape_earnings-mac64`
1. Create a `plist` file with this contents and place it in `~/Library/LaunchAgents` (replace `YOUR USER NAME HERE` with your actual username):

   ```xml
   <?xml version="1.0" encoding="UTF-8"?>
   <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
   <plist version="1.0">
   <dict>
      <key>Label</key>
      <string>com.jonathanbell.ffearnings</string>

      <key>ProgramArguments</key>
      <array>
         <string>/Users/YOUR USER NAME HERE/ff-earnings-dates-scraper/ff_scrape_earnings-mac64</string>
      </array>

      <key>RunAtLoad</key>
      <true/>

      <key>KeepAlive</key>
      <true/>

      <key>StandardErrorPath</key>
      <string>/Users/YOUR USER NAME HERE/ff-earnings-dates-scraper/log.txt</string>

      <key>StandardOutPath</key>
      <string>/Users/YOUR USER NAME HERE/ff-earnings-dates-scraper/log.txt</string>

      <key>WorkingDirectory</key>
      <string>/Users/YOUR USER NAME HERE/ff-earnings-dates-scraper</string>
   </dict>
   </plist>
   ```

1. Load the daemon
   - `launchctl load /path/to/your/plist/file.plist`
1. Start the daemon:
   - `launchctl start com.jonathanbell.ffearnings`
