import mimetypes
import os
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse, HTMLResponse
import uvicorn

app = FastAPI(title="PDF Uplink")

DEFAULT_FILENAME = os.getenv("UPLINK_FILENAME", "ai_sota_0001.pdf").strip() or "ai_sota_0001.pdf"
FORCED_MEDIA_TYPE = os.getenv("UPLINK_MEDIA_TYPE", "").strip()

# Resolve from this script location to avoid CWD-dependent 404s.
BASE_DIR = Path(__file__).resolve().parent
DOWNLOAD_ROUTE = "/download/{filename}"


def build_candidates(filename: str) -> list[Path]:
    safe_name = Path(filename).name
    return [
        BASE_DIR / "tmp" / safe_name,
        BASE_DIR / "worktree" / "develop" / "tmp" / safe_name,
    ]


@app.get("/", response_class=HTMLResponse)
def index() -> str:
    return (
        "<h1>PDF Uplink</h1>"
        f"<a href='/download/{DEFAULT_FILENAME}'>Tai {DEFAULT_FILENAME}</a>"
    )


@app.get(DOWNLOAD_ROUTE)
def download_file(filename: str) -> FileResponse:
    safe_name = Path(filename).name
    if safe_name != filename:
        raise HTTPException(status_code=400, detail="Invalid filename")

    candidates = build_candidates(safe_name)
    file_path = next((p for p in candidates if p.is_file()), None)
    if file_path is None:
        raise HTTPException(
            status_code=404,
            detail=f"File not found in candidates: {[str(p) for p in candidates]}",
        )

    media_type = FORCED_MEDIA_TYPE or mimetypes.guess_type(str(file_path))[0] or "application/octet-stream"

    return FileResponse(
        path=str(file_path),
        media_type=media_type,
        filename=safe_name,
    )


if __name__ == "__main__":
    uvicorn.run("fastapi_uplink:app", host="0.0.0.0", port=8000, reload=False)
