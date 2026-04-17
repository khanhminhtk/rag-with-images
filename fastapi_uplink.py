from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse, HTMLResponse
import uvicorn

app = FastAPI(title="PDF Uplink")

filename = "_page_1_Figure_0.jpeg"
type = "jpeg"

# Resolve from this script location to avoid CWD-dependent 404s.
BASE_DIR = Path(__file__).resolve().parent
PDF_CANDIDATES = [
    BASE_DIR / "tmp" / filename,
    BASE_DIR / "worktree" / "develop" / "tmp" / filename,
]
DOWNLOAD_ROUTE = "/download/{filename}"


@app.get("/", response_class=HTMLResponse)
def index() -> str:
    return (
        "<h1>PDF Uplink</h1>"
        f"<a href='{DOWNLOAD_ROUTE}'>Tai {filename}</a>"
    )


@app.get(DOWNLOAD_ROUTE)
def download_pdf() -> FileResponse:
    pdf_path = next((p for p in PDF_CANDIDATES if p.is_file()), None)
    if pdf_path is None:
        raise HTTPException(
            status_code=404,
            detail=f"File not found in candidates: {[str(p) for p in PDF_CANDIDATES]}",
        )

    return FileResponse(
        path=str(pdf_path),
        media_type=f"application/{type}",
        filename=filename,
    )


if __name__ == "__main__":
    uvicorn.run("fastapi_uplink:app", host="0.0.0.0", port=8000, reload=False)
