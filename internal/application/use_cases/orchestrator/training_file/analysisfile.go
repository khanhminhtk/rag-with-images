package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
)

func (T *trainingFileUseCase) validateAnalysisFileRequest(req *dtos.AnalysisFileRequest) (bool, error) {
	if req == nil {
		err := errors.New("request is nil")
		T.logger.Error("internal.application.use_cases.orchestrator.training_file.AnalysisFile invalid request", err)
		return false, err
	}

	if strings.TrimSpace(req.FilePath) == "" {
		err := errors.New("file_path is required")
		T.logger.Error("internal.application.use_cases.orchestrator.training_file.AnalysisFile missing file_path", err)
		return false, err
	}

	if strings.TrimSpace(req.Uuid) == "" {
		err := errors.New("uuid is required")
		T.logger.Error("internal.application.use_cases.orchestrator.training_file.AnalysisFile missing uuid", err)
		return false, err
	}

	if strings.TrimSpace(req.DistDir) == "" {
		err := errors.New("dist_dir is required")
		T.logger.Error("internal.application.use_cases.orchestrator.training_file.AnalysisFile missing dist_dir", err)
		return false, err
	}
	return true, nil
}

func (T *trainingFileUseCase) AnalysisFile(ctx context.Context, req *dtos.AnalysisFileRequest) (dtos.AnalysisFileResult, error) {
	status, err := T.validateAnalysisFileRequest(req)
	if err != nil {
		return dtos.AnalysisFileResult{Success: status}, err
	}

	scriptPath, err := resolveTrainingScriptPath()
	if err != nil {
		T.logger.Error("internal.application.use_cases.orchestrator.training_file.AnalysisFile resolve script path failed", err)
		return dtos.AnalysisFileResult{Success: false}, err
	}

	cmd := exec.CommandContext(
		ctx,
		"bash",
		scriptPath,
		"-pathfilesrc", req.FilePath,
		"-destdir", req.DistDir,
		"-dev", strconv.FormatBool(req.Dev),
	)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	outputText := strings.TrimSpace(string(output))
	if outputText != "" {
		T.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.AnalysisFile script output",
			"output", outputText,
			"script_path", scriptPath,
		)
	}

	if err != nil {
		wrappedErr := fmt.Errorf("analysis file script execution failed: %w", err)
		T.logger.Error(
			"internal.application.use_cases.orchestrator.training_file.AnalysisFile script execution failed",
			wrappedErr,
			"script_path", scriptPath,
			"file_path", req.FilePath,
			"dest_dir", req.DistDir,
			"dev", req.Dev,
		)
		return dtos.AnalysisFileResult{Success: false}, wrappedErr
	}

	T.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.AnalysisFile script execution succeeded",
		"script_path", scriptPath,
		"file_path", req.FilePath,
		"dest_dir", req.DistDir,
		"dev", req.Dev,
	)

	return dtos.AnalysisFileResult{Success: true}, nil
}

func resolveTrainingScriptPath() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot resolve caller path for analysisfile")
	}

	scriptPath := filepath.Join(filepath.Dir(currentFile), "marker_single_file.sh")
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute script path failed: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("script not found at %s: %w", absPath, err)
	}

	return absPath, nil
}
