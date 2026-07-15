sed -i '495s/w.ResponseRecorder.Header/w.Header/' internal/dashboard/server_test.go
sed -i '498s/w.ResponseRecorder.Body/w.Body/' internal/dashboard/server_test.go
