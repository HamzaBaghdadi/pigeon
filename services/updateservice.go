package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/updater"
	"github.com/HamzaBaghdadi/pigeon/internal/version"
)

// EventUpdateStatus carries UpdateStatus to the frontend.
const EventUpdateStatus = "update:status"

func init() {
	application.RegisterEvent[UpdateStatus](EventUpdateStatus)
}

const (
	updateFirstCheck = 15 * time.Second
	updateInterval   = 6 * time.Hour
)

// UpdateState is where the updater is.
type UpdateState string

const (
	UpdateDisabled    UpdateState = "disabled" // dev build or no signing key
	UpdateIdle        UpdateState = "idle"     // not checked yet
	UpdateChecking    UpdateState = "checking"
	UpdateUpToDate    UpdateState = "up-to-date"
	UpdateDownloading UpdateState = "downloading"
	UpdateReady       UpdateState = "ready"     // verified installer downloaded
	UpdateAvailable   UpdateState = "available" // portable copy: download manually
	UpdateError       UpdateState = "error"
)

// UpdateStatus is shown in Settings → About and the update banner.
type UpdateStatus struct {
	State       UpdateState `json:"state"`
	Current     string      `json:"current"`
	Latest      string      `json:"latest"`
	Notes       string      `json:"notes"`
	URL         string      `json:"url"`
	Error       string      `json:"error"`
	Reason      string      `json:"reason"` // why updates are disabled
	AutoCheck   bool        `json:"autoCheck"`
	Portable    bool        `json:"portable"`
	LastChecked time.Time   `json:"lastChecked"`
}

// UpdateService checks GitHub Releases for new versions and installs them.
type UpdateService struct {
	prefs *settings.Store
	up    *updater.Updater
	exe   string

	mu        sync.Mutex
	status    UpdateStatus
	installer string
	running   bool
	cancel    context.CancelFunc
}

// NewUpdateService wires the updater for this build.
func NewUpdateService(prefs *settings.Store) *UpdateService {
	exe, _ := os.Executable()
	s := &UpdateService{
		prefs: prefs,
		exe:   exe,
		up: &updater.Updater{
			APIURL:      updater.DefaultAPIURL,
			Current:     version.Version,
			PublicKey:   updater.PublicKey,
			AssetSuffix: updater.AssetSuffix,
			Dir:         filepath.Join(os.TempDir(), "Pigeon-updates"),
		},
	}
	s.status = UpdateStatus{State: UpdateIdle, Current: version.Version, AutoCheck: !prefs.Get().DisableAutoUpdate, Portable: !updater.Installed(exe)}
	switch {
	case !version.IsRelease():
		s.status.State, s.status.Reason = UpdateDisabled, "This is a development build, so it doesn't update itself."
	case updater.PublicKey == "":
		s.status.State, s.status.Reason = UpdateDisabled, "Updates aren't set up for this build."
	}
	return s
}

// ServiceStartup schedules background checks.
func (s *UpdateService) ServiceStartup(context.Context, application.ServiceOptions) error {
	if s.status.State == UpdateDisabled {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		timer := time.NewTimer(updateFirstCheck)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if !s.prefs.Get().DisableAutoUpdate {
				s.run(ctx)
			}
			timer.Reset(updateInterval)
		}
	}()
	return nil
}

// ServiceShutdown stops background checks.
func (s *UpdateService) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// GetUpdateStatus returns the current update status.
func (s *UpdateService) GetUpdateStatus() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// CheckNow checks for an update right away (and downloads it if found).
func (s *UpdateService) CheckNow() UpdateStatus {
	if s.GetUpdateStatus().State != UpdateDisabled {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		s.run(ctx)
	}
	return s.GetUpdateStatus()
}

// SetAutoCheck turns background update checks on or off.
func (s *UpdateService) SetAutoCheck(on bool) (UpdateStatus, error) {
	if _, err := s.prefs.Update(func(v *settings.Settings) { v.DisableAutoUpdate = !on }); err != nil {
		return s.GetUpdateStatus(), err
	}
	s.set(func(st *UpdateStatus) { st.AutoCheck = on })
	return s.GetUpdateStatus(), nil
}

// InstallAndRestart installs the downloaded update and restarts Pigeon.
func (s *UpdateService) InstallAndRestart() error {
	s.mu.Lock()
	installer, state := s.installer, s.status.State
	s.mu.Unlock()
	if state != UpdateReady || installer == "" {
		return errors.New("no update is ready to install")
	}
	if err := updater.Apply(installer, s.exe); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond) // let the call return to the UI
		if app := application.Get(); app != nil {
			app.Quit()
		}
	}()
	return nil
}

// run performs one check-and-download cycle; concurrent calls are coalesced.
func (s *UpdateService) run(ctx context.Context) {
	s.mu.Lock()
	if s.running || s.status.State == UpdateReady {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	s.set(func(st *UpdateStatus) { st.State, st.Error = UpdateChecking, "" })
	rel, err := s.up.Check(ctx)
	now := time.Now()
	if err != nil {
		s.set(func(st *UpdateStatus) { st.State, st.Error, st.LastChecked = UpdateError, err.Error(), now })
		return
	}
	if rel == nil {
		s.set(func(st *UpdateStatus) { st.State, st.LastChecked = UpdateUpToDate, now })
		return
	}
	s.set(func(st *UpdateStatus) {
		st.Latest, st.Notes, st.URL, st.LastChecked = rel.Version, rel.Notes, rel.URL, now
		st.State = UpdateDownloading
		if st.Portable {
			st.State = UpdateAvailable // can't replace itself; offer the download page
		}
	})
	if s.GetUpdateStatus().Portable {
		return
	}
	path, err := s.up.Download(ctx, rel)
	if err != nil {
		s.set(func(st *UpdateStatus) { st.State, st.Error = UpdateError, err.Error() })
		return
	}
	s.up.Cleanup(path)
	s.mu.Lock()
	s.installer = path
	s.mu.Unlock()
	s.set(func(st *UpdateStatus) { st.State = UpdateReady })
}

func (s *UpdateService) set(fn func(*UpdateStatus)) {
	s.mu.Lock()
	fn(&s.status)
	snap := s.status
	s.mu.Unlock()
	emit(EventUpdateStatus, snap)
}
