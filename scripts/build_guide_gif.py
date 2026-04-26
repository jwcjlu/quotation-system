from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parents[1]
FRAMES_DIR = ROOT / "web" / "src" / "assets" / "guide" / "gif-frames"
OUTPUT_GIF = ROOT / "web" / "src" / "assets" / "guide" / "getting-started.gif"


CAPTURE_STEPS = [
    {
        "file": "01-home.png",
        "title": "步骤 1",
        "body": "从 BOM 会话开始，点击右上角“新建 BOM 单”进入上传弹窗。",
        "duration": 1400,
    },
    {
        "file": "02-upload-dialog.png",
        "title": "步骤 2",
        "body": "在弹窗里选择平台、上传 Excel，并确认解析方式。",
        "duration": 2200,
    },
    {
        "file": "01-home.png",
        "title": "步骤 3",
        "body": "上传完成后回到会话列表，找到刚创建或已就绪的会话。",
        "duration": 1400,
    },
    {
        "file": "03-session-dashboard.png",
        "title": "步骤 4",
        "body": "点击“打开详情”，进入会话看板检查导入进度和平台勾选。",
        "duration": 1800,
    },
    {
        "file": "03-session-dashboard.png",
        "title": "步骤 5",
        "body": "重点查看 BOM 行、导入状态和单据信息，确认数据已经完整。",
        "duration": 2400,
    },
]


def load_font(size: int) -> ImageFont.ImageFont:
    candidates = [
        Path("C:/Windows/Fonts/msyh.ttc"),
        Path("C:/Windows/Fonts/msyhbd.ttc"),
        Path("C:/Windows/Fonts/simhei.ttf"),
        Path("C:/Windows/Fonts/segoeui.ttf"),
    ]
    for path in candidates:
        if path.exists():
            try:
                return ImageFont.truetype(str(path), size)
            except OSError:
                continue
    return ImageFont.load_default()


def wrap_text(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.ImageFont, max_width: int) -> list[str]:
    words = list(text)
    lines: list[str] = []
    current = ""
    for word in words:
        trial = current + word
        if draw.textlength(trial, font=font) <= max_width:
            current = trial
            continue
        if current:
            lines.append(current)
        current = word
    if current:
        lines.append(current)
    return lines


def annotate(image: Image.Image, title: str, body: str, progress: float) -> Image.Image:
    img = image.convert("RGBA")
    width, height = img.size
    overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)

    title_font = load_font(36)
    body_font = load_font(24)
    pill_font = load_font(22)

    draw.rounded_rectangle((32, 28, width - 32, 150), radius=24, fill=(15, 23, 42, 205))
    draw.rounded_rectangle((56, 52, 180, 92), radius=20, fill=(14, 165, 233, 255))
    draw.text((82, 60), "GIF 演示", fill=(255, 255, 255, 255), font=pill_font)
    draw.text((56, 100), title, fill=(255, 255, 255, 255), font=title_font)

    body_lines = wrap_text(draw, body, body_font, width - 128)
    draw.rounded_rectangle((32, height - 180, width - 32, height - 32), radius=28, fill=(255, 255, 255, 235))
    text_y = height - 150
    for line in body_lines:
        draw.text((56, text_y), line, fill=(15, 23, 42, 255), font=body_font)
        text_y += 34

    bar_x0, bar_y0, bar_x1, bar_y1 = 56, height - 70, width - 56, height - 52
    draw.rounded_rectangle((bar_x0, bar_y0, bar_x1, bar_y1), radius=10, fill=(226, 232, 240, 255))
    progress_x = bar_x0 + int((bar_x1 - bar_x0) * progress)
    draw.rounded_rectangle((bar_x0, bar_y0, progress_x, bar_y1), radius=10, fill=(14, 165, 233, 255))

    return Image.alpha_composite(img, overlay).convert("RGB")


def main() -> None:
    output_frames: list[Image.Image] = []
    durations: list[int] = []
    total = len(CAPTURE_STEPS)
    for idx, step in enumerate(CAPTURE_STEPS, start=1):
        frame_path = FRAMES_DIR / step["file"]
        if not frame_path.exists():
            raise FileNotFoundError(frame_path)
        with Image.open(frame_path) as raw:
            annotated = annotate(raw, step["title"], step["body"], idx / total)
            output_frames.append(annotated)
            durations.append(step["duration"])

    if not output_frames:
        raise RuntimeError("no frames generated")

    OUTPUT_GIF.parent.mkdir(parents=True, exist_ok=True)
    output_frames[0].save(
        OUTPUT_GIF,
        save_all=True,
        append_images=output_frames[1:],
        duration=durations,
        loop=0,
        optimize=False,
        disposal=2,
    )
    print(f"saved {OUTPUT_GIF}")


if __name__ == "__main__":
    main()
