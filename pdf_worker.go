package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Bornholm/amatl/pkg/command/cli/render"
	"github.com/Bornholm/amatl/pkg/pipeline"
	"github.com/kvitrvn/notevault/internal/vault"
)

const (
	pdfWorkerTimeout = 45 * time.Second
	pdfParentTimeout = 60 * time.Second
	maxPDFHTMLBytes  = 64 * 1024 * 1024
	maxPDFOutput     = 100 * 1024 * 1024
)

type detectedPDFBrowser struct {
	Name string
	Path string
}

func detectPDFBrowser() (detectedPDFBrowser, error) {
	return detectPDFBrowserWith(exec.LookPath)
}

func detectPDFBrowserWith(lookPath func(string) (string, error)) (detectedPDFBrowser, error) {
	candidates := []struct {
		command string
		name    string
	}{
		{command: "chromium", name: "Chromium"},
		{command: "chromium-browser", name: "Chromium"},
		{command: "google-chrome-stable", name: "Google Chrome"},
		{command: "google-chrome", name: "Google Chrome"},
	}
	for _, candidate := range candidates {
		path, err := lookPath(candidate.command)
		if err != nil {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		return detectedPDFBrowser{Name: candidate.name, Path: absolute}, nil
	}
	return detectedPDFBrowser{}, errors.New("Chromium ou Google Chrome est requis pour exporter en PDF")
}

func runInternalPDFWorker(args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("internal-render-pdf", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	browser := flags.String("browser", "", "")
	marginTop := flags.Float64("margin-top", 0, "")
	marginRight := flags.Float64("margin-right", 0, "")
	marginBottom := flags.Float64("margin-bottom", 0, "")
	marginLeft := flags.Float64("margin-left", 0, "")
	pageNumbers := flags.Bool("page-numbers", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("paramètres du worker PDF invalides")
	}
	if *browser == "" {
		return errors.New("navigateur PDF manquant")
	}
	for _, margin := range []float64{*marginTop, *marginRight, *marginBottom, *marginLeft} {
		if margin < 0.5 || margin > 4 {
			return errors.New("marge PDF invalide")
		}
	}
	html, err := io.ReadAll(io.LimitReader(input, maxPDFHTMLBytes+1))
	if err != nil {
		return errors.New("impossible de lire le document")
	}
	if len(html) > maxPDFHTMLBytes {
		return errors.New("document trop volumineux")
	}

	footer := ""
	if *pageNumbers {
		footer = `<div style="font-size:9px;width:100%;text-align:center"><span class="pageNumber"></span> / <span class="totalPages"></span></div>`
	}
	transformer := pipeline.Pipeline(render.PDFMiddleware(
		render.WithExecPath(*browser),
		render.WithTimeout(pdfWorkerTimeout),
		render.WithNoSandbox(false),
		render.WithBackground(true),
		render.WithMarginTop(*marginTop),
		render.WithMarginRight(*marginRight),
		render.WithMarginBottom(*marginBottom),
		render.WithMarginLeft(*marginLeft),
		render.WithDisplayFooterHeader(*pageNumbers),
		render.WithHeaderTemplate(""),
		render.WithFooterTemplate(footer),
	))
	payload := pipeline.NewPayload(html)
	if err := transformer.Transform(context.Background(), payload); err != nil {
		// Le parent ignore stderr afin de ne jamais exposer de chemin local ou
		// de détail Chromium dans l’interface. Conserver la cause ici rend le
		// test d’intégration directement diagnostiquable.
		return fmt.Errorf("échec du rendu Chromium : %w", err)
	}
	if !bytes.HasPrefix(payload.GetData(), []byte("%PDF-")) {
		return errors.New("sortie PDF invalide")
	}
	if _, err := output.Write(payload.GetData()); err != nil {
		return errors.New("impossible d’écrire le PDF")
	}
	return nil
}

func renderPDFInWorker(
	ctx context.Context,
	executable string,
	browser detectedPDFBrowser,
	document vault.PDFDocument,
) ([]byte, error) {
	command := exec.Command(executable, pdfWorkerArguments(browser, document)...)
	command.Stdin = bytes.NewReader(document.HTML)
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output limitedBuffer
	output.limit = maxPDFOutput
	command.Stdout = &output

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("démarrer le worker PDF : %w", err)
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
	}()
	select {
	case err := <-wait:
		if err != nil {
			killProcessGroup(command.Process.Pid)
			return nil, errors.New("le moteur PDF a échoué")
		}
	case <-ctx.Done():
		killProcessGroup(command.Process.Pid)
		<-wait
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("l’export PDF a dépassé le délai de 60 secondes")
		}
		return nil, ctx.Err()
	}
	if output.overflow {
		return nil, errors.New("le PDF généré est trop volumineux")
	}
	data := output.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, errors.New("le moteur n’a pas produit un PDF valide")
	}
	return append([]byte(nil), data...), nil
}

func pdfWorkerArguments(browser detectedPDFBrowser, document vault.PDFDocument) []string {
	return []string{
		"--internal-render-pdf",
		"--browser", browser.Path,
		"--margin-top", millimetersToCentimeters(document.Margins.Top),
		"--margin-right", millimetersToCentimeters(document.Margins.Right),
		"--margin-bottom", millimetersToCentimeters(document.Margins.Bottom),
		"--margin-left", millimetersToCentimeters(document.Margins.Left),
		"--page-numbers=" + strconv.FormatBool(document.PageNumbers),
	}
}

func millimetersToCentimeters(value float64) string {
	return strconv.FormatFloat(value/10, 'f', -1, 64)
}

func killProcessGroup(pid int) {
	if runtime.GOOS == "linux" && pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if b.overflow {
		return len(data), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.Buffer.Write(data[:remaining])
		b.overflow = true
		return len(data), nil
	}
	return b.Buffer.Write(data)
}

func isInternalPDFWorker(args []string) bool {
	return len(args) >= 2 && strings.EqualFold(args[1], "--internal-render-pdf")
}
