package cron

import "testing"

func TestJobOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  []JobOption
		wantSync bool
		wantTry  bool
	}{
		{
			name:     "默认选项",
			options:  nil,
			wantSync: false,
			wantTry:  false,
		},
		{
			name: "异步执行",
			options: []JobOption{
				WithAsync(true),
			},
			wantSync: true,
			wantTry:  false,
		},
		{
			name: "异常捕获",
			options: []JobOption{
				WithTryCatch(true),
			},
			wantSync: false,
			wantTry:  true,
		},
		{
			name: "组合选项",
			options: []JobOption{
				WithAsync(true),
				WithTryCatch(true),
			},
			wantSync: true,
			wantTry:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := NewJobModel("* * * * * *", func() {}, tt.options...)
			if err != nil {
				t.Fatal(err)
			}

			if job.async != tt.wantSync {
				t.Errorf("async = %v, want %v", job.async, tt.wantSync)
			}
			if job.tryCatch != tt.wantTry {
				t.Errorf("tryCatch = %v, want %v", job.tryCatch, tt.wantTry)
			}
		})
	}
}
