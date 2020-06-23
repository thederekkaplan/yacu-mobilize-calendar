export GOOGLE_APPLICATION_CREDENTIALS=dev-credentials.json
export ENV=DEV

test: 
	go run main.go

deploy:
	go mod tidy
	gcloud app deploy app.yaml --project commanding-way-273100