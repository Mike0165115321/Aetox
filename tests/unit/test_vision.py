import pytest
from unittest.mock import MagicMock, patch, mock_open
from pathlib import Path
from aetox.tools.vision import AetoxVision

@pytest.fixture
def vision_tool():
    return AetoxVision()

def test_vision_list_dir(vision_tool, tmp_path):
    # Create a dummy directory structure
    d = tmp_path / "test_dir"
    d.mkdir()
    (d / "file1.txt").write_text("hello")
    (d / "subdir").mkdir()
    
    result = vision_tool.execute({"action": "list", "path": str(d)})
    
    assert result["status"] == "success"
    assert "โครงสร้างโฟลเดอร์" in result["output"]
    assert "file1.txt" in result["output"]
    assert "subdir" in result["output"]

def test_vision_read_txt(vision_tool, tmp_path):
    f = tmp_path / "test.txt"
    f.write_text("Hello Thai: สวัสดี", encoding="utf-8")
    
    result = vision_tool.execute({"action": "read", "path": str(f)})
    
    assert result["status"] == "success"
    assert "Hello Thai: สวัสดี" in result["output"]
    assert result["raw_text"] == "Hello Thai: สวัสดี"

@patch("fitz.open")
def test_vision_read_pdf(mock_fitz_open, vision_tool, tmp_path):
    # Setup mock PDF
    mock_doc = MagicMock()
    mock_doc.__len__.return_value = 1
    mock_page = MagicMock()
    mock_page.get_text.return_value = "PDF Content"
    mock_doc.__getitem__.return_value = mock_page
    mock_fitz_open.return_value = mock_doc
    
    f = tmp_path / "test.pdf"
    f.touch() # Create empty file
    
    result = vision_tool.execute({"action": "read", "path": str(f)})
    
    assert result["status"] == "success"
    assert "PDF Content" in result["output"]
    mock_fitz_open.assert_called_once_with(str(f))

@patch("docx.Document")
def test_vision_read_docx(mock_docx_doc, vision_tool, tmp_path):
    # Setup mock Docx
    mock_para = MagicMock()
    mock_para.text = "Word Content"
    mock_doc = MagicMock()
    mock_doc.paragraphs = [mock_para]
    mock_docx_doc.return_value = mock_doc
    
    f = tmp_path / "test.docx"
    f.touch()
    
    result = vision_tool.execute({"action": "read", "path": str(f)})
    
    assert result["status"] == "success"
    assert "Word Content" in result["output"]

def test_vision_file_not_found(vision_tool):
    result = vision_tool.execute({"action": "read", "path": "non_existent.txt"})
    assert result["status"] == "failure"
    assert "ไม่พบตำแหน่ง" in result["error"]

def test_vision_unsupported_ext(vision_tool, tmp_path):
    f = tmp_path / "test.unknown"
    f.touch()
    result = vision_tool.execute({"action": "read", "path": str(f)})
    assert result["status"] == "failure"
    assert "ยังไม่อ่านไฟล์นามสกุล" in result["error"]
