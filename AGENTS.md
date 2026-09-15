# AGENTS.md

Đây là file hướng dẫn chính ở thư mục gốc của dự án.

Mọi AI code, coding agent hoặc lập trình viên tự động phải đọc file này trước khi sửa bất kỳ thứ gì trong dự án.

## Quy tắc bắt buộc

Trước khi code, phải đọc tài liệu chính trong thư mục `docs`.

Thứ tự đọc bắt buộc:

1. `docs/AGENTS.md`
2. `docs/PROJECT_OVERVIEW.md`
3. `docs/ARCHITECTURE.md`
4. `docs/FOLDER_STRUCTURE.md`
5. `docs/MODULES.md`
6. `docs/CODING_STANDARDS.md`
7. `docs/NAMING_CONVENTION.md`
8. `docs/FORMAT_AND_STYLE_GUIDE.md`

Nếu task liên quan sửa lỗi, phải đọc thêm:

1. `docs/TROUBLESHOOTING.md`
2. `docs/ERROR_KNOWLEDGE_BASE.md`
3. `docs/log/ERROR_FIX_LOG.md`
4. `docs/log/REPEATED_ERROR_LOG.md`
5. `docs/log/ROOT_CAUSE_ANALYSIS_LOG.md`
6. `docs/SKILL/FIX_BUG_SKILL.md`
7. `docs/SKILL/REPEAT_ERROR_FIX_SKILL.md`
8. `docs/SKILL/ROOT_CAUSE_ANALYSIS_SKILL.md`

Nếu task liên quan giao diện và dự án có thư mục `docs/design`, phải đọc thêm:

1. `docs/design/README.md`
2. `docs/design/DESIGN_SYSTEM.md`
3. `docs/design/UI_OVERVIEW.md`
4. `docs/design/VIEW_FILES_MAP.md`
5. `docs/design/UI_SYNC_RULES.md`
6. `docs/design/UI_DO_NOT_CHANGE.md`
7. `docs/SKILL/UI_SYNC_SKILL.md`

Nếu task liên quan API, phải đọc thêm:

1. `docs/API.md`
2. `docs/SKILL/ADD_API_SKILL.md`

Nếu task liên quan database, phải đọc thêm:

1. `docs/DATABASE.md`
2. `docs/SKILL/UPDATE_DATABASE_SKILL.md`

Nếu task liên quan cấu hình hoặc biến môi trường, phải đọc thêm:

1. `docs/CONFIGURATION.md`
2. `docs/ENVIRONMENT_VARIABLES.md`
3. `docs/SKILL/UPDATE_CONFIG_SKILL.md`

Nếu task liên quan đăng nhập, đăng xuất, session, token hoặc OAuth, phải đọc thêm:

1. `docs/AUTHENTICATION.md`
2. `docs/SECURITY.md`
3. `docs/SKILL/UPDATE_AUTH_SKILL.md`

Nếu task liên quan phân quyền, role hoặc permission, phải đọc thêm:

1. `docs/PERMISSIONS_AND_ROLES.md`
2. `docs/SECURITY.md`
3. `docs/SKILL/UPDATE_PERMISSION_SKILL.md`

Nếu task liên quan deploy, phải đọc thêm:

1. `docs/DEPLOYMENT.md`
2. `docs/RELEASE_CHECKLIST.md`
3. `docs/SKILL/DEPLOY_CHECK_SKILL.md`

## Quy tắc khi code

Không sửa lan man ngoài yêu cầu.
Không refactor lớn khi chưa được yêu cầu.
Không xóa code cũ khi chưa chắc chắn.
Không đổi cấu trúc dự án khi chưa có lý do rõ ràng.
Không sửa migration cũ đã chạy production.
Không sửa vendor, node_modules, cache, build output hoặc file generated.
Không commit file `.env` thật.
Không ghi secret, token, password, API key hoặc private key vào code, docs, log hoặc skill.
Không bỏ qua validation.
Không bỏ qua authorization.
Không tắt bảo mật để sửa lỗi nhanh.
Không che lỗi bằng try catch vô nghĩa.
Không đổi response API nếu có client đang dùng mà chưa đảm bảo tương thích.

## Quy tắc cập nhật tài liệu

Sau mỗi lần code, phải kiểm tra và cập nhật docs liên quan.

Nếu thêm tính năng, cập nhật `docs/FEATURES.md`.
Nếu thêm module, cập nhật `docs/MODULES.md`.
Nếu đổi cấu trúc thư mục, cập nhật `docs/FOLDER_STRUCTURE.md`.
Nếu đổi database, cập nhật `docs/DATABASE.md`.
Nếu đổi API, cập nhật `docs/API.md`.
Nếu đổi auth, cập nhật `docs/AUTHENTICATION.md`.
Nếu đổi permission, cập nhật `docs/PERMISSIONS_AND_ROLES.md`.
Nếu đổi security, cập nhật `docs/SECURITY.md`.
Nếu đổi config hoặc env, cập nhật `docs/CONFIGURATION.md` và `docs/ENVIRONMENT_VARIABLES.md`.
Nếu đổi test, cập nhật `docs/TESTING.md`.
Nếu đổi deploy, cập nhật `docs/DEPLOYMENT.md`.
Nếu đổi giao diện và có `docs/design`, cập nhật file phù hợp trong `docs/design`.
Nếu gặp lỗi có thể lặp lại, cập nhật `docs/TROUBLESHOOTING.md` và `docs/ERROR_KNOWLEDGE_BASE.md`.

## Quy tắc cập nhật log

Sau mỗi lần code, phải cập nhật `docs/log` nếu thư mục này tồn tại.

Luôn cập nhật:
1. `docs/log/ACTIVITY_LOG.md`

Nếu sửa code, cập nhật:
1. `docs/log/CODE_CHANGE_LOG.md`
2. `docs/log/FILE_CHANGE_LOG.md`

Nếu cập nhật docs, cập nhật:
1. `docs/log/DOCS_UPDATE_LOG.md`

Nếu chạy test hoặc không chạy được test, cập nhật:
1. `docs/log/TEST_LOG.md`

Nếu sửa lỗi, cập nhật:
1. `docs/log/ERROR_FIX_LOG.md`

Nếu lỗi lặp lại, cập nhật:
1. `docs/log/REPEATED_ERROR_LOG.md`

Nếu lỗi phức tạp, cập nhật:
1. `docs/log/ROOT_CAUSE_ANALYSIS_LOG.md`

Nếu có cách sửa thất bại, cập nhật:
1. `docs/log/FAILED_ATTEMPT_LOG.md`

Nếu sửa giao diện và có UI log, cập nhật log UI phù hợp.

## Quy tắc dùng skill

Trước khi code, phải đọc skill phù hợp trong `docs/SKILL` nếu thư mục này tồn tại.

Nếu thêm tính năng, đọc `docs/SKILL/ADD_FEATURE_SKILL.md`.
Nếu sửa lỗi, đọc `docs/SKILL/FIX_BUG_SKILL.md`.
Nếu lỗi đã từng gặp, đọc `docs/SKILL/REPEAT_ERROR_FIX_SKILL.md`.
Nếu cần phân tích nguyên nhân gốc, đọc `docs/SKILL/ROOT_CAUSE_ANALYSIS_SKILL.md`.
Nếu thêm module, đọc `docs/SKILL/ADD_MODULE_SKILL.md`.
Nếu thêm hoặc sửa API, đọc `docs/SKILL/ADD_API_SKILL.md`.
Nếu sửa database, đọc `docs/SKILL/UPDATE_DATABASE_SKILL.md`.
Nếu sửa config, đọc `docs/SKILL/UPDATE_CONFIG_SKILL.md`.
Nếu sửa auth, đọc `docs/SKILL/UPDATE_AUTH_SKILL.md`.
Nếu sửa quyền, đọc `docs/SKILL/UPDATE_PERMISSION_SKILL.md`.
Nếu sửa giao diện, đọc `docs/SKILL/UI_SYNC_SKILL.md`.
Sau khi code, đọc `docs/SKILL/UPDATE_DOCS_SKILL.md`.
Sau khi code, đọc `docs/SKILL/UPDATE_LOG_SKILL.md`.
Trước khi báo hoàn thành, đọc `docs/SKILL/TEST_BEFORE_COMMIT_SKILL.md`.

## Quy tắc xử lý lỗi đã từng gặp

Khi gặp lỗi, không được sửa mò ngay.

Phải kiểm tra lỗi cũ trước:
1. `docs/log/ERROR_FIX_LOG.md`
2. `docs/log/REPEATED_ERROR_LOG.md`
3. `docs/log/ROOT_CAUSE_ANALYSIS_LOG.md`
4. `docs/log/FAILED_ATTEMPT_LOG.md`
5. `docs/TROUBLESHOOTING.md`
6. `docs/ERROR_KNOWLEDGE_BASE.md`

Nếu lỗi đã từng gặp, phải đối chiếu:
1. Thông báo lỗi
2. File gây lỗi
3. Module liên quan
4. Lệnh gây lỗi
5. Nguyên nhân gốc
6. Cách sửa cũ
7. Cách test cũ

Nếu nguyên nhân giống, ưu tiên dùng cách sửa đã được ghi lại.

Nếu nguyên nhân khác, phải ghi lại biến thể lỗi mới.

Nếu lỗi mới, sau khi sửa xong phải ghi lại đầy đủ:
1. Tên lỗi
2. Nhóm lỗi
3. Dấu hiệu nhận biết
4. Thông báo lỗi
5. File liên quan
6. Module liên quan
7. Nguyên nhân gốc
8. Cách sửa đúng
9. Cách không nên sửa
10. Cách test lại
11. Cách phòng tránh

## Quy tắc giao diện nếu dự án có UI

Nếu dự án có `docs/design`, mọi thay đổi giao diện phải đọc `docs/design` trước.

Không được tạo style lệch hệ thống.
Không được tạo component trùng.
Không được sửa global style nếu chỉ cần sửa một component.
Không được phá responsive.
Không được phá sidebar, navbar, dropdown, form, table, modal, loading, empty state hoặc error state.
Không được đổi màu, font, spacing, radius, shadow nếu chưa được yêu cầu.
Sau khi sửa giao diện, phải cập nhật docs/design liên quan và docs/log liên quan.

## Trước khi báo hoàn thành

Phải kiểm tra:
1. Code đã đúng yêu cầu
2. Không sửa ngoài phạm vi
3. Không phá chức năng cũ
4. Không ghi secret
5. Đã chạy test, build, lint hoặc kiểm tra thủ công phù hợp
6. Đã cập nhật docs liên quan
7. Đã cập nhật docs/log liên quan
8. Đã cập nhật docs/design nếu có sửa giao diện
9. Đã ghi lỗi nếu có lỗi mới hoặc lỗi lặp lại
10. Đã báo cáo rõ file code đã sửa, file docs đã cập nhật, file log đã cập nhật và skill đã đọc
