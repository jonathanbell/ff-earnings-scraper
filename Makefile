compile:
	chmod +x ./scripts/build.sh && ./scripts/build.sh

reset:
	rm -rf ./bin/*
	rm -f log.txt
