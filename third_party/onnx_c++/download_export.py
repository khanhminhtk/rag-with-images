import importlib.util
from pathlib import Path

import torch
import transformers
from transformers import AutoModel, AutoTokenizer

torch.set_default_device("cpu")

if importlib.util.find_spec("onnx") is None:
    raise RuntimeError(
        "Missing dependency: `onnx`.\n"
        "Install it in your current environment, e.g.\n"
        "  python -m pip install onnx"
    )

if int(transformers.__version__.split(".", 1)[0]) >= 5:
    raise RuntimeError(
        "download_export.py requires transformers 4.x for jinaai/jina-clip-v1. "
        "Install a compatible version, e.g. `python -m pip install \"transformers==4.46.3\"`."
    )

model_name = 'jinaai/jina-clip-v1'
tokenizer = AutoTokenizer.from_pretrained(
    model_name,
    trust_remote_code=True,
)
model = AutoModel.from_pretrained(
    model_name,
    trust_remote_code=True,
    low_cpu_mem_usage=False,
)
model = model.to("cpu")
model.eval()

vision_model = model.vision_model
text_model = model.text_model

project_dir = Path(__file__).resolve().parent
onnx_output_dir = project_dir / "model" / "onnx"
onnx_output_dir.mkdir(parents=True, exist_ok=True)

tokenizer_output_dir = project_dir / "model" / "tokenizer"
tokenizer_output_dir.mkdir(parents=True, exist_ok=True)
tokenizer.save_pretrained(tokenizer_output_dir)

dummy_image = torch.randn(1, 3, 224, 224, device="cpu")

torch.onnx.export(
    vision_model,
    (dummy_image,),
    str(onnx_output_dir / "jina_vision.onnx"),
    export_params=True,
    opset_version=14,
    do_constant_folding=True,
    dynamo=False,
    input_names=['pixel_values'],
    output_names=['image_embeds'],
    dynamic_axes={
        'pixel_values': {0: 'batch_size'},
        'image_embeds': {0: 'batch_size'}
    }
)

dummy_input_ids = torch.randint(0, 1000, (1, 1024), device="cpu")
dummy_attention_mask = torch.ones(1, 1024, dtype=torch.long, device="cpu")

torch.onnx.export(
    text_model,
    (dummy_input_ids, dummy_attention_mask),
    str(onnx_output_dir / "jina_text.onnx"),
    export_params=True,
    opset_version=14,
    do_constant_folding=True,
    dynamo=False,
    input_names=['input_ids', 'attention_mask'],
    output_names=['text_embeds'],
    dynamic_axes={
        'input_ids': {0: 'batch_size', 1: 'sequence_length'},
        'attention_mask': {0: 'batch_size', 1: 'sequence_length'},
        'text_embeds': {0: 'batch_size'}
    }
)

print(f"Exported ONNX models to: {onnx_output_dir}")
print(f"Exported Tokenizer to: {tokenizer_output_dir}")
