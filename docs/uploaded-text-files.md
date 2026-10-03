# Uploaded text files

Clients can upload attachments with `POST /v1/files` and reference their IDs in
Responses `input_file` parts or Chat Completions `file` parts. The proxy resolves
the uploaded bytes before forwarding a model request.

Textual uploads are expanded into `input_text` (Responses) or `text` (Chat
Completions), prefixed with their original filename. UTF-8 content, including
Unicode and line breaks, is preserved; an optional UTF-8 BOM is removed. Invalid
UTF-8 or NUL-containing text is rejected with HTTP 400 before invoking the model.
There is no text truncation, embedding, retrieval, or vector collection involved.

The text classification covers `text/*`, common JSON/XML/YAML/TOML/JavaScript/SQL
media types, and known text/source extensions when the upload has a generic MIME
type. Binary documents such as PDF continue to use base64 file input. This does
not add document parsing or image support to providers that do not support them.
Explicit inline `file_data` is not rewritten by the hosted-file resolver.

Previously all uploaded files were sent as binary file input. In a live check,
the GPT route accepted PDF files but silently ignored TXT files, returning a
completed response that said no attachment was present. Text expansion allows
these models to receive the actual uploaded text without relying on their binary
document parser. The client still uses the hosted upload and file ID workflow.

The same adaptation permits text-only models to read textual uploads. It does not
make PDFs supported on the Abliteration GLM route: that provider's Anthropic
compatibility surface rejects `document` content blocks. A model's text modality
and its API's document-input support are separate capabilities.

File lifecycle is unchanged. Successfully handled requests consume uploaded
files; a later DELETE can consequently return 404. Rejected requests retain the
file. A 404 alone is not proof of either successful reading or failed uploading.

Regression tests cover both API formats, Unicode and BOM content, mixed text/PDF
ordering, binary byte preservation, invalid-text rejection, and consumption after
successful requests. For an end-to-end check, put a random code only in the
uploaded document and verify that the final model answer returns that code.
