# docker-iperf-app

## About
docker-iperf-app is a containerized application that runs the iperf and ping command every hour to collect network statistics. The statistics are served on /api/speedtest/latest for integration with dashboards. The data can be optionally stored in MYSQL/MARIADB.

## Setup
Clone the repository

    git clone git@github.com:RedPine404/docker-iperf-app.git

Set the values for config.json

    {
        "iperf_server_ip": "198.51.100.2",
        "use_db":  true/false,
        "db_user": "iperfuser",
        "db_pass": "changeme",
        "db_host": "localhost",
        "db_port": 3306,
        "db_name": "iperfapp"
    }

 You can alternatively use docker environment variables

    environment:
        - IPERF_SERVER_IP=198.51.100.2
        - USE_DB=true
        - MYSQL_USER=iperfuser
        - MYSQL_PASSWORD=changeme
        - DB_HOST=localhost
        - DB_PORT=3306
        - MYSQL_DATABASE=iperfapp
 
## Build
    docker build -t iperf-app .

## Run
    docker run -p 8000:8000 --env-file db.env iperf-app

## Export/Import    
    docker save -o iperf-app.tar iperf-app:latest

    docker load -i iperf-app.tar
