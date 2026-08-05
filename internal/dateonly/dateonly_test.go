package dateonly

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalJSON(t *testing.T) {
	d := New(2026, time.August, 15)

	b, err := json.Marshal(d)

	require.NoError(t, err)
	assert.Equal(t, `"2026-08-15"`, string(b), "must be YYYY-MM-DD, NOT RFC3339 (e.g. must NOT contain T00:00:00Z)")
}

func TestMarshalJSON_NilPointerFieldMarshalsToNull(t *testing.T) {
	type wrapper struct {
		D *Date `json:"d"`
	}

	b, err := json.Marshal(wrapper{D: nil})

	require.NoError(t, err)
	assert.JSONEq(t, `{"d":null}`, string(b))
}

func TestUnmarshalJSON(t *testing.T) {
	var d Date

	err := json.Unmarshal([]byte(`"2026-08-15"`), &d)

	require.NoError(t, err)
	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.August, d.Month())
	assert.Equal(t, 15, d.Day())
	assert.Equal(t, time.UTC, d.Location())
}

func TestUnmarshalJSON_RejectsRFC3339(t *testing.T) {
	var d Date

	err := json.Unmarshal([]byte(`"2026-08-15T00:00:00Z"`), &d)

	assert.Error(t, err, "must reject full RFC3339 - only bare YYYY-MM-DD is accepted")
}

func TestUnmarshalJSON_Null(t *testing.T) {
	type wrapper struct {
		D *Date `json:"d"`
	}
	var w wrapper

	err := json.Unmarshal([]byte(`{"d":null}`), &w)

	require.NoError(t, err)
	assert.Nil(t, w.D)
}

// TestFromTime_NonUTCLocation_DoesNotShiftDate is the explicit proof asked
// for: constructing a Date from a time.Time in a non-UTC location must
// yield the SAME calendar date the caller intended, and the resulting
// Date's internal Time must be UTC - this is the invariant every other
// dateonly function relies on to stay drift-free.
func TestFromTime_NonUTCLocation_DoesNotShiftDate(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)

	testCases := []struct {
		name  string
		input time.Time
		wantY int
		wantM time.Month
		wantD int
	}{
		{
			name:  "WIB just after midnight - still same calendar day in WIB",
			input: time.Date(2026, time.August, 15, 0, 30, 0, 0, wib),
			wantY: 2026, wantM: time.August, wantD: 15,
		},
		{
			name:  "WIB late evening - still same calendar day in WIB",
			input: time.Date(2026, time.August, 15, 23, 30, 0, 0, wib),
			wantY: 2026, wantM: time.August, wantD: 15,
		},
		{
			name:  "negative UTC offset (US Pacific, UTC-8)",
			input: time.Date(2026, time.August, 15, 1, 0, 0, 0, time.FixedZone("PST", -8*60*60)),
			wantY: 2026, wantM: time.August, wantD: 15,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := FromTime(tc.input)

			assert.Equal(t, tc.wantY, d.Year())
			assert.Equal(t, tc.wantM, d.Month())
			assert.Equal(t, tc.wantD, d.Day())
			assert.Equal(t, time.UTC, d.Location(), "internal Time must always be re-anchored to UTC, regardless of input location")
			assert.Equal(t, 0, d.Hour())
			assert.Equal(t, 0, d.Minute())
			assert.Equal(t, 0, d.Second())
		})
	}
}

func TestNew_AlwaysUTC(t *testing.T) {
	d := New(2026, time.February, 1)

	assert.Equal(t, time.UTC, d.Location())
	assert.True(t, d.Time.Equal(time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)))
}

func TestParse(t *testing.T) {
	d, err := Parse("2026-08-15")

	require.NoError(t, err)
	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.August, d.Month())
	assert.Equal(t, 15, d.Day())
	assert.Equal(t, time.UTC, d.Location())
}

func TestParse_InvalidFormat(t *testing.T) {
	_, err := Parse("15/08/2026")

	require.Error(t, err)
}

func TestString(t *testing.T) {
	d := New(2026, time.August, 5)

	assert.Equal(t, "2026-08-05", d.String())
}

// TestRoundTrip_MarshalThenUnmarshal proves MarshalJSON/UnmarshalJSON are
// symmetric - the exact concern behind pgconv.ToDate/FromDate now routing
// through this type instead of bare time.Time.
func TestRoundTrip_MarshalThenUnmarshal(t *testing.T) {
	original := New(2026, time.December, 31)

	b, err := json.Marshal(original)
	require.NoError(t, err)

	var roundTripped Date
	require.NoError(t, json.Unmarshal(b, &roundTripped))

	assert.True(t, original.Time.Equal(roundTripped.Time))
}
