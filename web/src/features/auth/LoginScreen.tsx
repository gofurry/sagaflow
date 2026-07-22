import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Form, Input, Space, Typography } from 'antd'
import type { AuthStatus } from '../../api/types'

interface Props { status: AuthStatus; loading: boolean; error?: string; onLogin: (username: string, password: string) => void; onSetup: (username: string, displayName: string, password: string) => void }
export function LoginScreen({ status, loading, error, onLogin, onSetup }: Props) {
  return <div className="login-shell">
    <Card className="login-card" variant="borderless">
      <Space orientation="vertical" size={22} style={{ width: '100%' }}>
        <div className="login-heading">
          <Typography.Title level={2} style={{ margin: 0 }}>SagaFlow</Typography.Title>
          <Typography.Text type="secondary">AI 漫剧资产生产与画布编排</Typography.Text>
        </div>
        {error && <Alert type="error" showIcon message={error}/>}
		{!status.initialized ? <Form initialValues={{ username: 'admin', display_name: 'Creator' }} layout="vertical" onFinish={(values) => onSetup(values.username, values.display_name, values.password)} requiredMark={false}>
		  <Form.Item name="display_name" label="显示名称" rules={[{ required: true }]}><Input autoFocus size="large" prefix={<UserOutlined/>}/></Form.Item>
		  <Form.Item name="username" label="登录用户名" rules={[{ required: true }]}><Input size="large" prefix={<UserOutlined/>}/></Form.Item>
		  <Form.Item name="password" label="设置密码" rules={[{ required: true, min: 8 }]}><Input.Password size="large" prefix={<LockOutlined/>}/></Form.Item>
		  <Button block htmlType="submit" loading={loading} size="large" type="primary">初始化本地工作台</Button>
		</Form> : <Form initialValues={{ username: 'admin' }} layout="vertical" onFinish={(values) => onLogin(values.username, values.password)} requiredMark={false}>
		  <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}><Input autoFocus size="large" prefix={<UserOutlined/>} placeholder="请输入用户名"/></Form.Item>
		  <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password size="large" prefix={<LockOutlined/>} placeholder="请输入密码"/>
          </Form.Item>
          <Button block htmlType="submit" loading={loading} size="large" type="primary">进入工作台</Button>
        </Form>}
      </Space>
    </Card>
  </div>
}

export function PasswordChangeScreen({ loading, error, onSubmit }: { loading: boolean; error?: string; onSubmit: (currentPassword: string,newPassword: string)=>void }) {
	return <div className="login-shell"><Card className="login-card" variant="borderless"><Space orientation="vertical" size={22} style={{width:'100%'}}><div className="login-heading"><Typography.Title level={2} style={{margin:0}}>设置新密码</Typography.Title><Typography.Text type="secondary">当前使用的是临时密码，修改后才能进入工作台。</Typography.Text></div>{error&&<Alert type="error" showIcon message={error}/>}<Form layout="vertical" onFinish={(values)=>onSubmit(values.current_password,values.new_password)}><Form.Item label="当前密码" name="current_password" rules={[{required:true}]}><Input.Password prefix={<LockOutlined/>}/></Form.Item><Form.Item label="新密码" name="new_password" rules={[{required:true,min:6}]}><Input.Password prefix={<LockOutlined/>}/></Form.Item><Button block htmlType="submit" loading={loading} size="large" type="primary">保存新密码</Button></Form></Space></Card></div>
}
