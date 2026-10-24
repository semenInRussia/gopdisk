import { useLayoutEffect, useRef } from "react";
import { createPortal } from "react-dom";
import type { StoredFile } from "./filesSlice";

interface Props {
    files: StoredFile[];
    anchor: DOMRect | null;
    onConfirm: () => void;
    onCancel: () => void;
}

export default function DeleteConfirmation({
    files,
    anchor,
    onConfirm,
    onCancel,
}: Props) {
    const dialog = useRef<HTMLDialogElement>(null);
    const cancel = useRef<HTMLButtonElement>(null);

    useLayoutEffect(() => {
        const element = dialog.current;
        if (!element) return;
        element.showModal();
        const position = () => {
            const { width, height } = element.getBoundingClientRect();
            const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
            const viewportHeight = window.innerHeight;
            const left = anchor ? anchor.right - width : (viewportWidth - width) / 2;
            const below = (anchor?.bottom ?? 0) + 8;
            const top = anchor
                ? (below + height <= viewportHeight - 16 ? below : anchor.top - height - 8)
                : (viewportHeight - height) / 2;
            element.style.left = `${Math.max(16, Math.min(left, viewportWidth - width - 16))}px`;
            element.style.top = `${Math.max(16, Math.min(top, viewportHeight - height - 16))}px`;
        };
        position();
        cancel.current?.focus({ preventScroll: true });
        window.addEventListener('resize', position);
        return () => window.removeEventListener('resize', position);
    }, [anchor]);

    return createPortal(
        <dialog
            ref={dialog}
            id="file-delete-dialog"
            className="p-desktop-modal active file-delete-dialog"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="delete-title"
            aria-describedby="delete-description"
            onCancel={(event) => {
                event.preventDefault();
                dialog.current?.close("cancel");
            }}
            onClose={() => {
                if (dialog.current?.returnValue === "delete") onConfirm();
                else onCancel();
            }}
        >
            <div className="p-desktop-modal__surface">
                <div className="p-desktop-modal__body">
                    <div className="p-desktop-modal__title" id="delete-title" role="heading" aria-level={2}>
                        {files.length === 1
                            ? "Удалить файл?"
                            : `Удалить файлы (${files.length})?`}
                    </div>
                    <div
                        className="p-desktop-modal__description"
                        id="delete-description"
                    >
                        {files.length === 1
                            ? `«${files[0].name}» будет удалён. `
                            : "Выбранные файлы будут удалены. "}
                        Это действие нельзя отменить.
                    </div>
                    {files.length > 1 && (
                        <div className="p-desktop-modal__description">
                            {files
                                .slice(0, 5)
                                .map((file) => file.name)
                                .join(", ")}
                            {files.length > 5 &&
                                `; ещё файлов: ${files.length - 5}`}
                        </div>
                    )}
                </div>
                <div className="p-desktop-modal__actions">
                    <button
                        ref={cancel}
                        className="p-desktop-modal__action p-desktop-modal__action--secondary"
                        type="button"
                        onClick={() => dialog.current?.close("cancel")}
                    >
                        Отмена
                    </button>
                    <button
                        className="p-desktop-modal__action p-desktop-modal__action--destructive"
                        type="button"
                        onClick={() => dialog.current?.close("delete")}
                    >
                        Удалить
                    </button>
                </div>
            </div>
        </dialog>,
        document.body,
    );
}
