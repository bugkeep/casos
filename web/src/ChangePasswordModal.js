import React from "react";
import {Form, Input, Modal} from "antd";
import {LockOutlined} from "@ant-design/icons";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";

const passwordRules = [
  {required: true},
  {validator: (_, value) => {
    const bytes = value ? new TextEncoder().encode(value).length : 0;
    return !value || (bytes >= 12 && bytes <= 72) ? Promise.resolve() : Promise.reject(new Error("Use 12 to 72 bytes"));
  }},
];

function ChangePasswordModal({open, onClose, onChanged}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = React.useState(false);

  const submit = async() => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const response = await AccountBackend.changePassword(values.currentPassword, values.newPassword);
      if (response.status !== "ok") {
        Setting.showMessage("error", response.msg || "Password change failed");
        return;
      }
      form.resetFields();
      Setting.showMessage("success", "Password changed");
      await onChanged();
      onClose();
    } catch {
      Setting.showMessage("error", "Unable to reach the CasOS service");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal title="Change password" open={open} onCancel={onClose} onOk={submit} confirmLoading={submitting} okText="Change password" destroyOnHidden>
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item label="Current password" name="currentPassword" rules={[{required: true}]}>
          <Input.Password prefix={<LockOutlined />} autoComplete="current-password" autoFocus />
        </Form.Item>
        <Form.Item label="New password" name="newPassword" rules={passwordRules}>
          <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          label="Confirm password"
          name="confirmPassword"
          dependencies={["newPassword"]}
          rules={[
            {required: true},
            ({getFieldValue}) => ({validator(_, value) {
              return !value || getFieldValue("newPassword") === value ? Promise.resolve() : Promise.reject(new Error("Passwords do not match"));
            }}),
          ]}>
          <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  );
}

export default ChangePasswordModal;
