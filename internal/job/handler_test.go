package job

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/auth/authtest"
	"github.com/nathan/cnc-pm-backend/internal/dateonly"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// newTestRouter simulasikan router di belakang RequireAuth sebagai role
// owner (tanpa batasan RBAC apapun, lihat isJobHiddenFromCaller) - cocok
// untuk test lama yang menguji perilaku umum (bukan scoping RBAC itu
// sendiri), supaya tidak perlu tahu-menahu soal identitas pemanggil.
func newTestRouter(repo Repository) *gin.Engine {
	return newTestRouterAs(repo, user.RoleOwner, uuid.New())
}

// newTestRouterAs: sama seperti newTestRouter, tapi dengan role/userID
// pemanggil eksplisit - dipakai test RBAC scoping (keputusan 2026-10-03)
// yang justru menguji identitas pemanggilnya.
func newTestRouterAs(repo Repository, role user.Role, userID uuid.UUID) *gin.Engine {
	svc := newTestService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	rg.Use(authtest.Middleware(userID, role))
	h.RegisterRoutes(rg)
	return r
}

// TestGetByID_ScheduledDate_SerializesAsDateOnly is the JSON-body-literal
// regression guard for the date-serialization bug (docs/api-contract.md
// Catatan Desain: "Kenapa field bertipe tanggal sekarang serialize sebagai
// YYYY-MM-DD..."): jobs.scheduled_date must appear as "YYYY-MM-DD" in the
// ACTUAL marshaled HTTP response body - a Go-level *dateonly.Date/*time.Time
// assertion (like every other test in this package) can't catch this class
// of bug at all, since it never inspects the JSON bytes.
func TestGetByID_ScheduledDate_SerializesAsDateOnly(t *testing.T) {
	repo := newFakeRepository()
	router := newTestRouter(repo)

	scheduled := dateonly.New(2026, time.August, 15)
	created, err := repo.Create(context.Background(), Job{
		CustomerID: uuid.New(), Title: "Servis rutin", ScheduledDate: &scheduled,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"scheduled_date":"2026-08-15"`, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}

// TestGetByID_CompletedDate_SerializesAsDateOnly: same guard for
// completed_date (set automatically by UpdateStatus, not by client input -
// see service.go).
func TestGetByID_CompletedDate_SerializesAsDateOnly(t *testing.T) {
	repo := newFakeRepository()
	router := newTestRouter(repo)
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusCompleted, uuid.New(), nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Regexp(t, `"completed_date":"\d{4}-\d{2}-\d{2}"`, body, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}

// ===========================================================================
// RBAC scoping: teknisi cuma lihat/ubah job yang di-assign ke dirinya
// (keputusan Nathan 2026-10-03). Job yang belum di-assign siapapun
// (technician_id NULL) SENGAJA ikut tersembunyi dari teknisi - bukan tetap
// kelihatan untuk "diambil". Lihat isJobHiddenFromCaller.
// ===========================================================================

func TestList_Teknisi_OnlySeesOwnJobs(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	ctx := context.Background()
	technicianA := uuid.New()

	own, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job milik A"})
	require.NoError(t, err)
	repo.jobs[own.ID] = setTechnician(repo.jobs[own.ID], technicianA)

	othersJob, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job milik teknisi lain"})
	require.NoError(t, err)
	repo.jobs[othersJob.ID] = setTechnician(repo.jobs[othersJob.ID], uuid.New())

	_, err = svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job belum di-assign"})
	require.NoError(t, err)

	router := newTestRouterAs(repo, user.RoleTeknisi, technicianA)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var parsed struct {
		Data []Job `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1, "teknisi cuma boleh lihat job miliknya - bukan job teknisi lain, bukan job yang belum di-assign")
	assert.Equal(t, own.ID, parsed.Data[0].ID)
}

func TestList_Teknisi_CannotOverrideScopeViaQueryParam(t *testing.T) {
	// GET /jobs tidak pernah menerima technician_id dari query sama sekali
	// (lihat Handler.List - tidak ada c.Query("technician_id")), jadi
	// mengirimnya tidak boleh berefek apapun - scoping tetap dipaksa dari
	// JWT claim pemanggil, bukan bisa "diminta" jadi technician lain.
	repo := newFakeRepository()
	svc := newTestService(repo)
	ctx := context.Background()
	technicianA := uuid.New()
	technicianB := uuid.New()

	own, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job milik A"})
	require.NoError(t, err)
	repo.jobs[own.ID] = setTechnician(repo.jobs[own.ID], technicianA)

	othersJob, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job milik B"})
	require.NoError(t, err)
	repo.jobs[othersJob.ID] = setTechnician(repo.jobs[othersJob.ID], technicianB)

	router := newTestRouterAs(repo, user.RoleTeknisi, technicianA)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?technician_id="+technicianB.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var parsed struct {
		Data []Job `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1, "query param technician_id harus diabaikan total untuk role teknisi")
	assert.Equal(t, own.ID, parsed.Data[0].ID)
}

func TestList_OwnerAdmin_StillSeeAllJobs(t *testing.T) {
	// Regresi: scoping RBAC baru ini TIDAK BOLEH memengaruhi owner/admin -
	// keduanya tetap melihat semua job seperti sebelum keputusan 2026-10-03.
	repo := newFakeRepository()
	svc := newTestService(repo)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job A"})
	require.NoError(t, err)
	assigned, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Job B"})
	require.NoError(t, err)
	repo.jobs[assigned.ID] = setTechnician(repo.jobs[assigned.ID], uuid.New())

	for _, role := range []user.Role{user.RoleOwner, user.RoleAdmin} {
		router := newTestRouterAs(repo, role, uuid.New())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "role %s", role)
		var parsed struct {
			Data []Job `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
		assert.Len(t, parsed.Data, 2, "role %s harus tetap lihat semua job", role)
	}
}

func TestGetByID_Teknisi_OwnJob_Returns200(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	technicianA := uuid.New()
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Job milik A"})
	require.NoError(t, err)
	repo.jobs[created.ID] = setTechnician(repo.jobs[created.ID], technicianA)

	router := newTestRouterAs(repo, user.RoleTeknisi, technicianA)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetByID_Teknisi_OthersJob_Returns404NotForbidden(t *testing.T) {
	// 404, BUKAN 403 - supaya tidak membocorkan ke teknisi bahwa job ini ada
	// tapi bukan miliknya (keputusan eksplisit 2026-10-03).
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Job milik orang lain"})
	require.NoError(t, err)
	repo.jobs[created.ID] = setTechnician(repo.jobs[created.ID], uuid.New())

	router := newTestRouterAs(repo, user.RoleTeknisi, uuid.New())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), `"NOT_FOUND"`))
}

func TestGetByID_Teknisi_UnassignedJob_Returns404(t *testing.T) {
	// Keputusan eksplisit 2026-10-03: job yang belum di-assign siapapun ikut
	// TERSEMBUNYI dari teknisi, bukan tetap kelihatan untuk "diambil".
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Belum di-assign"})
	require.NoError(t, err)

	router := newTestRouterAs(repo, user.RoleTeknisi, uuid.New())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetByID_OwnerAdmin_SeeAnyJob(t *testing.T) {
	// Regresi: owner/admin tidak kena scoping ini sama sekali.
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Job siapapun"})
	require.NoError(t, err)
	repo.jobs[created.ID] = setTechnician(repo.jobs[created.ID], uuid.New())

	for _, role := range []user.Role{user.RoleOwner, user.RoleAdmin} {
		router := newTestRouterAs(repo, role, uuid.New())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "role %s", role)
	}
}

func TestUpdateStatus_Teknisi_OwnJob_Succeeds(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	technicianA := uuid.New()
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Job milik A"})
	require.NoError(t, err)
	repo.jobs[created.ID] = setTechnician(repo.jobs[created.ID], technicianA)

	router := newTestRouterAs(repo, user.RoleTeknisi, technicianA)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+created.ID.String()+"/status",
		strings.NewReader(`{"status":"in_progress"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateStatus_Teknisi_OthersJob_Returns404NotForbidden(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Job milik orang lain"})
	require.NoError(t, err)
	repo.jobs[created.ID] = setTechnician(repo.jobs[created.ID], uuid.New())

	router := newTestRouterAs(repo, user.RoleTeknisi, uuid.New())
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+created.ID.String()+"/status",
		strings.NewReader(`{"status":"in_progress"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	// Pastikan status BENAR-BENAR tidak berubah (bukan cuma response code
	// yang salah tapi tulisannya tetap lolos).
	unchanged, err := svc.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusRequested, unchanged.Status)
}

func TestUpdateStatus_Teknisi_UnassignedJob_Returns404(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Belum di-assign"})
	require.NoError(t, err)

	router := newTestRouterAs(repo, user.RoleTeknisi, uuid.New())
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+created.ID.String()+"/status",
		strings.NewReader(`{"status":"in_progress"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
