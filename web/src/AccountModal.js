import React from "react";
import {Form, Input, Modal} from "antd";
import i18next from "i18next";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";

// AccountModal edits the built-in account: its display name, and its password
// when both password fields are filled in. It is only reachable while CasOS runs
// without Casdoor, since a Casdoor account is edited in Casdoor.
function AccountModal({account, open, onCancel, onUpdateAccount}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = React.useState(false);

  React.useEffect(() => {
    if (open) {
      form.setFieldsValue({displayName: account?.displayName || "", currentPassword: "", newPassword: "", confirmPassword: ""});
    }
  }, [open, account, form]);

  const onOk = async() => {
    let values;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }

    setSubmitting(true);
    try {
      const res = await AccountBackend.updateAccount({
        displayName: values.displayName,
        avatar: account?.avatar || "",
        currentPassword: values.currentPassword || "",
        newPassword: values.newPassword || "",
      });
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      Setting.showMessage("success", i18next.t("general:Successfully saved"));
      onUpdateAccount?.();
      onCancel();
    } catch (error) {
      Setting.showMessage("error", error.message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={i18next.t("account:My Account")}
      open={open}
      okText={i18next.t("general:Save")}
      cancelText={i18next.t("general:Cancel")}
      confirmLoading={submitting}
      onCancel={onCancel}
      onOk={onOk}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" autoComplete="off">
        <Form.Item name="displayName" label={i18next.t("general:Display name")}>
          <Input />
        </Form.Item>
        <Form.Item
          name="currentPassword"
          label={i18next.t("account:Old Password")}
          dependencies={["newPassword"]}
          rules={[
            ({getFieldValue}) => ({
              validator(_, value) {
                return getFieldValue("newPassword") && !value
                  ? Promise.reject(new Error(i18next.t("account:Please input your password")))
                  : Promise.resolve();
              },
            }),
          ]}
        >
          <Input.Password placeholder={i18next.t("account:Enter current password")} autoComplete="current-password" />
        </Form.Item>
        <Form.Item name="newPassword" label={i18next.t("account:New password")}>
          <Input.Password placeholder={i18next.t("account:Enter new password")} autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="confirmPassword"
          label={i18next.t("account:Confirm password")}
          dependencies={["newPassword"]}
          rules={[
            ({getFieldValue}) => ({
              validator(_, value) {
                return getFieldValue("newPassword") === (value || "")
                  ? Promise.resolve()
                  : Promise.reject(new Error(i18next.t("account:Passwords do not match")));
              },
            }),
          ]}
        >
          <Input.Password placeholder={i18next.t("account:Enter new password")} autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  );
}

export default AccountModal;
