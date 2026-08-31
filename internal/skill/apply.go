package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// renameOp performs the rename steps of a transaction. It is a variable so
// tests can inject apply and restore failures.
var renameOp = os.Rename

const (
	stagingDirName = ".clime-staging"
	backupDirName  = ".clime-backup"
)

// targetName maps a dot-directory name to a display-friendly target name.
var targetName = map[string]string{
	".claude": "claude",
	".codex":  "codex",
}

// agentTargets returns the agent base directories (~/.claude, ~/.codex) that
// exist on this machine.
func agentTargets() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	var targets []string
	for _, dir := range []string{".claude", ".codex"} {
		base := filepath.Join(home, dir)
		if info, err := os.Stat(base); err == nil && info.IsDir() {
			targets = append(targets, base)
		}
	}
	return targets, nil
}

// InstalledTargets returns the display names of agent targets where the named
// skill directory currently exists.
func InstalledTargets(name string) []string {
	targets, err := agentTargets()
	if err != nil {
		return nil
	}
	var installed []string
	for _, base := range targets {
		if _, err := os.Stat(filepath.Join(base, "skills", name)); err == nil {
			installed = append(installed, targetName[filepath.Base(base)])
		}
	}
	return installed
}

// plannedSkill is one skill prepared during preflight: its name plus the
// validated content directory inside an immutable snapshot.
type plannedSkill struct {
	name   string
	srcDir string
}

// preflight validates the complete manifest, ensures every referenced
// snapshot is cached (contacting the remote only for uncached versions), and
// resolves every selected skill to validated content. Nothing is mutated.
func preflight(m *Manifest) ([]plannedSkill, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var plan []plannedSkill
	for _, r := range m.Repos {
		snapDir, err := EnsureSnapshot(r.ID, r.Version)
		if err != nil {
			return nil, err
		}
		catalog, err := ReadCatalog(snapDir)
		if err != nil {
			return nil, fmt.Errorf("repository %s at %s: %w", r.ID.Canonical(), r.Version, err)
		}
		for _, name := range r.Skills {
			entry, ok := catalog.Find(name)
			if !ok {
				return nil, fmt.Errorf("skill %q does not exist in %s at version %s", name, r.ID.Canonical(), r.Version)
			}
			srcDir, err := skillContentDir(snapDir, entry)
			if err != nil {
				return nil, fmt.Errorf("repository %s at %s: %w", r.ID.Canonical(), r.Version, err)
			}
			plan = append(plan, plannedSkill{name: name, srcDir: srcDir})
		}
	}
	return plan, nil
}

// undoOp reverses one applied filesystem step.
type undoOp struct {
	remove  string // directory to delete (content that was newly placed)
	renameA string // backup to move back...
	renameB string // ...to its original location
}

// PartialStateError reports a failed transaction whose automatic restoration
// also failed. Backups referenced by the error still exist on disk.
type PartialStateError struct {
	Cause       error
	RestoreErr  error
	BackupPaths []string
}

func (e *PartialStateError) Error() string {
	return fmt.Sprintf("apply failed (%v) and automatic restoration also failed (%v); agent targets may be in a partial state, backups are retained at: %s",
		e.Cause, e.RestoreErr, strings.Join(e.BackupPaths, ", "))
}

func (e *PartialStateError) Unwrap() error { return e.Cause }

// transaction tracks the staging and backup directories of one apply pass.
type transaction struct {
	journal     []undoOp
	cleanupDirs []string
	backupRoots []string
}

func (tx *transaction) rollback() error {
	var errs []error
	for i := len(tx.journal) - 1; i >= 0; i-- {
		op := tx.journal[i]
		if op.remove != "" {
			if err := os.RemoveAll(op.remove); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		if op.renameA != "" {
			if err := renameOp(op.renameA, op.renameB); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	tx.commit()
	return nil
}

func (tx *transaction) commit() {
	for _, dir := range tx.cleanupDirs {
		os.RemoveAll(dir)
	}
}

// apply installs every planned skill into each agent target and removes the
// named skills, staging new content and backing up replaced directories so a
// failure can be rolled back.
func (tx *transaction) apply(targets []string, plan []plannedSkill, removed []string) error {
	type stagedTarget struct {
		skillsRoot string
		staging    string
		backup     string
	}
	var staged []stagedTarget

	// Stage all content first so replacement is a sequence of renames.
	for _, base := range targets {
		skillsRoot := filepath.Join(base, "skills")
		if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
			return err
		}
		st := stagedTarget{
			skillsRoot: skillsRoot,
			staging:    filepath.Join(skillsRoot, stagingDirName),
			backup:     filepath.Join(skillsRoot, backupDirName),
		}
		// Drop leftovers from an interrupted earlier run.
		os.RemoveAll(st.staging)
		os.RemoveAll(st.backup)
		if err := os.MkdirAll(st.staging, 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(st.backup, 0o755); err != nil {
			return err
		}
		tx.cleanupDirs = append(tx.cleanupDirs, st.staging, st.backup)
		tx.backupRoots = append(tx.backupRoots, st.backup)
		for _, p := range plan {
			if err := copyTree(p.srcDir, filepath.Join(st.staging, p.name)); err != nil {
				return fmt.Errorf("failed to stage skill %q: %w", p.name, err)
			}
		}
		staged = append(staged, st)
	}

	// Replace target directories.
	for _, st := range staged {
		for _, p := range plan {
			final := filepath.Join(st.skillsRoot, p.name)
			backup := filepath.Join(st.backup, p.name)
			if _, err := os.Stat(final); err == nil {
				if err := renameOp(final, backup); err != nil {
					return fmt.Errorf("failed to back up %s: %w", final, err)
				}
				tx.journal = append(tx.journal, undoOp{renameA: backup, renameB: final})
			}
			if err := renameOp(filepath.Join(st.staging, p.name), final); err != nil {
				return fmt.Errorf("failed to install %s: %w", final, err)
			}
			tx.journal = append(tx.journal, undoOp{remove: final})
			// Merge the undo pair: removing the new dir must precede
			// restoring the backup, which reverse-order replay ensures.
		}
		for _, name := range removed {
			final := filepath.Join(st.skillsRoot, name)
			if _, err := os.Stat(final); err != nil {
				continue
			}
			backup := filepath.Join(st.backup, name)
			if err := renameOp(final, backup); err != nil {
				return fmt.Errorf("failed to remove %s: %w", final, err)
			}
			tx.journal = append(tx.journal, undoOp{renameA: backup, renameB: final})
		}
	}
	return nil
}

// Reconcile applies the manifest's desired state to every agent target as one
// transaction: preflight resolves and validates everything before any target
// changes, staging and backups make replacement recoverable, and the manifest
// is saved (when save is true) only after targets applied successfully. On
// failure every changed target is restored from backup automatically.
// removed lists skill directories to delete from targets. It returns the
// display names of the agent targets that were applied.
func Reconcile(m *Manifest, removed []string, save bool) ([]string, error) {
	plan, err := preflight(m)
	if err != nil {
		return nil, err
	}
	targets, err := agentTargets()
	if err != nil {
		return nil, err
	}

	tx := &transaction{}
	fail := func(cause error) error {
		if restoreErr := tx.rollback(); restoreErr != nil {
			return &PartialStateError{Cause: cause, RestoreErr: restoreErr, BackupPaths: tx.backupRoots}
		}
		return cause
	}

	if err := tx.apply(targets, plan, removed); err != nil {
		return nil, fail(err)
	}
	if save {
		if err := m.Save(); err != nil {
			return nil, fail(fmt.Errorf("failed to save skills manifest: %w", err))
		}
	}
	tx.commit()

	names := make([]string, 0, len(targets))
	for _, base := range targets {
		names = append(names, targetName[filepath.Base(base)])
	}
	return names, nil
}
