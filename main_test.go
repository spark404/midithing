package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_removeMarkers(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"Test <> markers",
			args{
				"<blub>",
			},
			"blub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removeMarkers(tt.args.s); got != tt.want {
				t.Errorf("removeMarkers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	type args struct {
		rawMessage string
	}
	tests := []struct {
		name    string
		args    args
		want    *GrblStatus
		wantErr bool
	}{
		{
			"Test with only MPos status",
			args{
				rawMessage: "<Idle|MPos:0.400,0.000,0.333|FS:0,0>",
			},
			&GrblStatus{
				state: "Idle",
				position: struct {
					X float64
					Y float64
					Z float64
				}{
					X: float64(0.400),
					Y: float64(0.000),
					Z: float64(0.333),
				},
			},
			false,
		},
		{
			"Test with Mpos and WCO status",
			args{
				rawMessage: "<Idle|MPos:0.000,0.000,0.000|FS:0,0|WCO:0.000,0.000,0.000>",
			},
			&GrblStatus{
				state: "Idle",
				position: struct {
					X float64
					Y float64
					Z float64
				}{
					X: float64(0.000),
					Y: float64(0.000),
					Z: float64(0.000),
				},
			},
			false,
		},
		{
			"Test with invalid status string",
			args{
				rawMessage: "<Idle",
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.args.rawMessage)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				assert.Equal(t, tt.want, got)
				return
			}

			assert.Equal(t, tt.want.state, got.state)
			assert.InDelta(t, tt.want.position.X, got.position.X, 0.0001)
			assert.InDelta(t, tt.want.position.Y, got.position.Y, 0.0001)
			assert.InDelta(t, tt.want.position.Z, got.position.Z, 0.0001)
		})
	}
}
